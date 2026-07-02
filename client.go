package mezon

import (
	"errors"
	"log"
	"sync"
	"time"
)

// Default connection settings, port of the constants in MezonClientCore.ts.
const (
	DefaultHost      = "gw.mezon.ai"
	DefaultPort      = "443"
	DefaultUseSSL    = true
	DefaultTimeoutMs = 7000
)

// ClientConfig configures a MezonClient. BotID and Token are required.
type ClientConfig struct {
	BotID   string
	Token   string
	Host    string
	Port    string
	UseSSL  *bool // nil => default (true)
	Timeout time.Duration

	// Store, when non-nil, is an L2 cache shared across bot instances (e.g.
	// Redis). Channel and user lookups consult it before hitting the REST API
	// and populate it on a miss, so replicas avoid redundant REST calls. The
	// in-memory caches remain the L1 (live objects); Store only holds the
	// serializable data needed to rebuild them. nil => in-memory only.
	Store SharedStore
	// CacheTTL bounds how long entries live in Store (<= 0 => 5 minutes).
	CacheTTL time.Duration
	// MaxUsersCache / MaxChannelsCache bound the in-memory L1 caches so a
	// long-running bot does not grow unboundedly (<= 0 => 5000 each).
	MaxUsersCache    int
	MaxChannelsCache int

	// MessageStore, when non-nil, persists every inbound message and backs the
	// per-channel Messages cache on a miss (port of the SQLite MessageDatabase).
	// A SQLite implementation is available in the messagedb submodule. nil =>
	// messages are cached in memory only.
	MessageStore MessageStore
}

// MezonClient is the high-level Mezon bot client, port of MezonClient +
// MezonClientCore. Construct with NewMezonClient, register event handlers with
// On*/On, then call Login.
type MezonClient struct {
	Token    string
	ClientID string
	Host     string
	Port     string
	UseSSL   bool
	Timeout  time.Duration

	Clans    *CacheManager[string, *Clan]
	Channels *CacheManager[string, *TextChannel]
	Users    *CacheManager[string, *User]

	loginBasePath  string
	apiClient      *MezonApi
	socket         *DefaultSocket
	channelManager *ChannelManager
	session        *Session
	queue          *AsyncThrottleQueue
	events         *emitter
	store          SharedStore
	messageDB      MessageStore
	cacheTTL       time.Duration

	mu             sync.Mutex
	internalsBound bool
	hardDisconnect bool
	reconnecting   bool
}

// NewMezonClient creates a client from config, applying defaults.
func NewMezonClient(cfg ClientConfig) (*MezonClient, error) {
	if cfg.BotID == "" {
		return nil, errors.New("botId is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("token is required")
	}
	host := cfg.Host
	if host == "" {
		host = DefaultHost
	}
	port := cfg.Port
	if port == "" {
		port = DefaultPort
	}
	useSSL := DefaultUseSSL
	if cfg.UseSSL != nil {
		useSSL = *cfg.UseSSL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeoutMs * time.Millisecond
	}
	scheme := "http://"
	if useSSL {
		scheme = "https://"
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	maxUsers := cfg.MaxUsersCache
	if maxUsers <= 0 {
		maxUsers = defaultMaxCache
	}
	maxChannels := cfg.MaxChannelsCache
	if maxChannels <= 0 {
		maxChannels = defaultMaxCache
	}
	c := &MezonClient{
		Token:         cfg.Token,
		ClientID:      cfg.BotID,
		Host:          host,
		Port:          port,
		UseSSL:        useSSL,
		Timeout:       timeout,
		loginBasePath: scheme + host + ":" + port,
		queue:         NewAsyncThrottleQueue(0),
		events:        newEmitter(),
		store:         cfg.Store,
		messageDB:     cfg.MessageStore,
		cacheTTL:      cacheTTL,
	}
	c.Clans = NewCacheManager[string, *Clan](func(string) (*Clan, error) { return nil, ErrNotFound }, 0)
	c.Channels = NewCacheManager[string, *TextChannel](c.fetchChannel, maxChannels)
	c.Users = NewCacheManager[string, *User](c.fetchUser, maxUsers)
	return c, nil
}

// On registers a handler for an event (see the Event* constants). The payload
// type depends on the event; for EventChannelMessage it is *ChannelMessage,
// for other events it is the decoded protobuf message pointer.
func (c *MezonClient) On(event string, h Handler) { c.events.on(event, h) }

// OnChannelMessage registers a handler for inbound chat messages.
func (c *MezonClient) OnChannelMessage(h func(*ChannelMessage)) {
	c.events.on(EventChannelMessage, func(p any) {
		if m, ok := p.(*ChannelMessage); ok {
			h(m)
		}
	})
}

// OnReady registers a handler invoked once the client has logged in and joined.
func (c *MezonClient) OnReady(h func()) {
	c.events.on(EventReady, func(any) { h() })
}

func (c *MezonClient) initManagers(basePath string, session *Session) {
	c.apiClient = NewMezonApi(c.Token, basePath, c.Timeout)
	wsURL := ""
	if session != nil {
		wsURL = session.WsURL
	}
	c.socket = NewDefaultSocket(wsURL, c.Host, c.Port, c.UseSSL, c.events.emit)
	c.socket.OnDisconnect = func(reason string) {
		if c.hardDisconnect {
			return
		}
		go c.retryConnect()
	}
	c.channelManager = newChannelManager(c.apiClient, c.socket, c)
	c.bindInternalListeners()
}

// Login authenticates the bot, connects the socket and joins all clans, port of
// MezonClientCore.login/handleClientLogin. It returns once the client is ready.
func (c *MezonClient) Login() error {
	c.hardDisconnect = false
	tempApi := NewMezonApi(c.Token, c.loginBasePath, c.Timeout)
	sessApi, err := tempApi.MezonAuthenticate(c.ClientID, c.Token)
	if err != nil {
		return err
	}
	if sessApi == nil || sessApi.Token == "" {
		return errors.New("authenticate returned empty session")
	}
	session, err := NewSession(sessApi.Token, sessApi.RefreshToken, itoaID(sessApi.UserId), sessApi.ApiUrl, sessApi.IdToken, sessApi.WsUrl)
	if err != nil {
		return err
	}
	c.session = session

	basePath := c.loginBasePath
	if sessApi.ApiUrl != "" {
		host, port, useSSL, perr := ParseURLToHostAndSSL(sessApi.ApiUrl)
		if perr == nil {
			c.Host, c.Port, c.UseSSL = host, port, useSSL
			scheme := "http://"
			if useSSL {
				scheme = "https://"
			}
			basePath = scheme + host + ":" + port
		}
	}
	c.initManagers(basePath, session)

	if err := c.socket.Connect(session, true); err != nil {
		return err
	}
	if err := c.connectSocket(session.Token); err != nil {
		return err
	}
	if err := c.channelManager.InitAllDMChannels(session.Token); err != nil {
		// non-fatal, mirror TS which logs and continues
		log.Printf("mezon: InitAllDMChannels failed: %v", err)
	}
	// Pre-cache discovered DM channels as live TextChannels, port of
	// MezonClientCore._initDmChannelCache.
	c.initDmChannelCache()
	c.events.emit(EventReady, nil)
	return nil
}

// connectSocket joins clan chats and builds Clan objects, port of
// socket_manager.connectSocket.
func (c *MezonClient) connectSocket(sessionToken string) error {
	clans, err := c.apiClient.ListClanDescs(sessionToken, 0, 0, "")
	if err != nil {
		return err
	}
	type clanInfo struct {
		id, name, welcome string
	}
	list := make([]clanInfo, 0)
	for _, cl := range clans.GetClandesc() {
		list = append(list, clanInfo{itoaID(cl.ClanId), cl.ClanName, itoaID(cl.WelcomeChannelId)})
	}
	list = append(list, clanInfo{"0", "", ""}) // global / DM pseudo-clan
	for _, cl := range list {
		if _, err := c.socket.JoinClanChat(cl.id); err != nil {
			// TS aborts connectSocket on a join failure; we log and continue
			// joining the rest so one bad clan does not block the whole bot.
			log.Printf("mezon: JoinClanChat(%s) failed: %v", cl.id, err)
		}
		time.Sleep(50 * time.Millisecond)
		if _, ok := c.Clans.Get(cl.id); !ok {
			clanObj := newClan(cl.id, orString(cl.name, "unknown"), cl.welcome, cl.name, c, sessionToken)
			c.Clans.Set(cl.id, clanObj)
		}
	}
	return nil
}

func (c *MezonClient) retryConnect() {
	c.mu.Lock()
	if c.reconnecting || c.hardDisconnect {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	delay := 5 * time.Second
	const maxDelay = 60 * time.Second
	for !c.hardDisconnect {
		time.Sleep(delay)
		if c.session == nil {
			break
		}
		if err := c.socket.Connect(c.session, true); err == nil {
			if err := c.connectSocket(c.session.Token); err == nil {
				break
			} else {
				log.Printf("mezon: reconnect connectSocket failed: %v", err)
			}
		} else {
			log.Printf("mezon: reconnect dial failed, retrying in %s: %v", delay*2, err)
		}
		if delay = delay * 2; delay > maxDelay {
			delay = maxDelay
		}
	}
	c.mu.Lock()
	c.reconnecting = false
	c.mu.Unlock()
}

// Close shuts down the socket and prevents reconnection.
func (c *MezonClient) Close() {
	c.hardDisconnect = true
	if c.socket != nil {
		c.socket.Close()
	}
}

// Socket returns the underlying socket for advanced/raw realtime calls.
func (c *MezonClient) Socket() *DefaultSocket { return c.socket }

// ChannelManager returns the DM channel manager.
func (c *MezonClient) ChannelManager() *ChannelManager { return c.channelManager }

func orString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
