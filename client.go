package mezon

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/quangledang23/mezon-sdk-go/api"
)

// Default connection settings, port of the constants in MezonClientCore.ts.
const (
	DefaultHost      = "gw.mezon.ai"
	DefaultPort      = "443"
	DefaultUseSSL    = true
	DefaultTimeoutMs = 7000
)

const (
	// sessionExpiryBuffer is how close to its expiry a session token is
	// already treated as expired and renewed (ensureValidSession's default
	// bufferSec in MezonClientCore.ts).
	sessionExpiryBuffer = 30 * time.Second
	// healthyEndpointMinInterval rate-limits the gateway healthy-endpoint
	// lookup (HEALTHY_ENDPOINT_MIN_INTERVAL_MS).
	healthyEndpointMinInterval = 15 * time.Second
)

// Reconnect backoff bounds: retries start at reconnectInitialDelay and double
// up to reconnectMaxDelay. Variables so tests can shorten them.
var (
	reconnectInitialDelay = 5 * time.Second
	reconnectMaxDelay     = 60 * time.Second
)

// ClientConfig configures a MezonClient. BotID and Token are required.
type ClientConfig struct {
	BotID   string
	Token   string
	Host    string
	Port    string
	UseSSL  *bool // nil => default (true)
	Timeout time.Duration

	// Transport selects the realtime wire transport: TransportTCP (abridged
	// TLS/TCP, the default when empty) or TransportWebSocket.
	Transport TransportKind
	// TLSInsecureSkipVerify disables certificate verification for the TCP
	// transport; dev gateways use self-signed certificates.
	TLSInsecureSkipVerify bool

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

	Transport             TransportKind
	TLSInsecureSkipVerify bool

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

	// mu guards session, hardDisconnect, reconnecting, reauth and
	// lastHealthyEndpointAsk. session is replaced, never modified in place, so
	// a *Session read under mu stays consistent after the lock is released.
	mu                     sync.Mutex
	internalsBound         bool
	hardDisconnect         bool
	reconnecting           bool
	reauth                 *reauthCall
	lastHealthyEndpointAsk time.Time
}

// reauthCall is one in-flight re-authentication shared by every caller that
// needs a fresh session while it runs (reAuthPromise in MezonClientCore.ts).
type reauthCall struct {
	done    chan struct{}
	session *Session
	err     error
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
		Token:                 cfg.Token,
		ClientID:              cfg.BotID,
		Host:                  host,
		Port:                  port,
		UseSSL:                useSSL,
		Timeout:               timeout,
		Transport:             cfg.Transport,
		TLSInsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		loginBasePath:         scheme + host + ":" + port,
		queue:                 NewAsyncThrottleQueue(0),
		events:                newEmitter(),
		store:                 cfg.Store,
		messageDB:             cfg.MessageStore,
		cacheTTL:              cacheTTL,
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
	c.socket.Transport = c.Transport
	c.socket.TLSInsecureSkipVerify = c.TLSInsecureSkipVerify
	if session != nil {
		c.socket.TcpURL = session.TcpURL
	}
	// REST API calls prefer the realtime socket once it is open (see
	// MezonApi.doRawSocket); until then they go over HTTP.
	c.apiClient.socket = c.socket
	c.socket.OnDisconnect = func(reason string) {
		if c.isHardDisconnected() {
			return
		}
		log.Printf("mezon: socket disconnected (%s), scheduling reconnect", reason)
		go c.retryConnect()
	}
	c.channelManager = newChannelManager(c.apiClient, c.socket, c)
	c.bindInternalListeners()
}

// currentSession returns the session in use, nil before Login.
func (c *MezonClient) currentSession() *Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

func (c *MezonClient) setSession(s *Session) {
	c.mu.Lock()
	c.session = s
	c.mu.Unlock()
}

func (c *MezonClient) isHardDisconnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hardDisconnect
}

// authenticate requests a new session from the login gateway.
func (c *MezonClient) authenticate() (*Session, error) {
	tempApi := NewMezonApi(c.Token, c.loginBasePath, c.Timeout)
	sessApi, err := tempApi.MezonAuthenticate(c.ClientID, c.Token)
	if err != nil {
		return nil, err
	}
	if sessApi == nil || sessApi.Token == "" {
		return nil, errors.New("authenticate returned empty session")
	}
	return sessionFromProto(sessApi)
}

// apiBasePath maps a session api_url to the scheme://host:port form MezonApi
// uses. ok is false when the URL is empty or unparseable.
func apiBasePath(apiURL string) (basePath, host, port string, useSSL, ok bool) {
	if apiURL == "" {
		return "", "", "", false, false
	}
	host, port, useSSL, err := ParseURLToHostAndSSL(apiURL)
	if err != nil {
		return "", "", "", false, false
	}
	scheme := "http://"
	if useSSL {
		scheme = "https://"
	}
	return scheme + host + ":" + port, host, port, useSSL, true
}

// Login authenticates the bot, connects the socket and joins all clans, port of
// MezonClientCore.login/handleClientLogin. It returns once the client is ready.
func (c *MezonClient) Login() error {
	c.mu.Lock()
	c.hardDisconnect = false
	c.mu.Unlock()
	session, err := c.authenticate()
	if err != nil {
		return err
	}
	c.setSession(session)

	basePath := c.loginBasePath
	if next, host, port, useSSL, ok := apiBasePath(session.APIURL); ok {
		c.Host, c.Port, c.UseSSL = host, port, useSSL
		basePath = next
	}
	c.initManagers(basePath, session)

	if err := c.socket.Connect(session, true); err != nil {
		return err
	}
	if err := c.postConnectInit(session.Token); err != nil {
		return err
	}
	c.events.emit(EventReady, nil)
	return nil
}

// IsSessionExpired reports whether the session token is expired or expires
// within buffer, port of MezonClientCore.isSessionExpired. It is true before
// Login.
func (c *MezonClient) IsSessionExpired(buffer time.Duration) bool {
	session := c.currentSession()
	if session == nil {
		return true
	}
	return session.IsExpired(time.Now().Add(buffer).Unix())
}

// EnsureValidSession returns a session whose token stays valid for at least
// 30 seconds, re-authenticating the bot first when the current one is expired
// or about to expire, port of MezonClientCore.ensureValidSession (mezon-sdk
// v2.8.65). The client and its clans call it before every REST request, so a
// long-running bot keeps working past the lifetime of its first token; call it
// directly for a token to pass to Api().
func (c *MezonClient) EnsureValidSession() (*Session, error) {
	return c.ensureValidSession(sessionExpiryBuffer, false)
}

// ensureValidSession is EnsureValidSession with an explicit buffer; force
// re-authenticates even when the current token still looks valid. Concurrent
// callers share one authenticate round trip.
func (c *MezonClient) ensureValidSession(buffer time.Duration, force bool) (*Session, error) {
	c.mu.Lock()
	if s := c.session; !force && s != nil && !s.IsExpired(time.Now().Add(buffer).Unix()) {
		c.mu.Unlock()
		return s, nil
	}
	if call := c.reauth; call != nil {
		c.mu.Unlock()
		<-call.done
		return call.session, call.err
	}
	call := &reauthCall{done: make(chan struct{})}
	c.reauth = call
	c.mu.Unlock()

	call.session, call.err = c.reAuthenticate()

	c.mu.Lock()
	c.reauth = nil
	c.mu.Unlock()
	close(call.done)
	return call.session, call.err
}

// reAuthenticate swaps in a freshly authenticated session, port of
// SessionManager.reAuthenticate. The new session keeps whatever the server
// left blank (endpoint URLs, id token, refresh token) from the previous one.
func (c *MezonClient) reAuthenticate() (*Session, error) {
	fresh, err := c.authenticate()
	if err != nil {
		return nil, fmt.Errorf("re-authenticate failed: %w", err)
	}
	return c.adoptSession(fresh), nil
}

// adoptSession makes fresh the client's session, filling whatever it leaves
// blank from the previous one and handing its token to every cached clan. It
// is how a session replaces another after Login, whether it came from a
// re-authentication or from a RefreshSessionEvent pushed by the server.
func (c *MezonClient) adoptSession(fresh *Session) *Session {
	c.mu.Lock()
	if old := c.session; old != nil {
		fresh.UserID = old.UserID
		fresh.CreatedAt = old.CreatedAt
		if fresh.RefreshToken == "" {
			fresh.RefreshToken, fresh.RefreshExpiresAt = old.RefreshToken, old.RefreshExpiresAt
		}
		if fresh.APIURL == "" {
			fresh.APIURL = old.APIURL
		}
		if fresh.WsURL == "" {
			fresh.WsURL = old.WsURL
		}
		if fresh.TcpURL == "" {
			fresh.TcpURL = old.TcpURL
		}
		if fresh.IDToken == "" {
			fresh.IDToken = old.IDToken
		}
	}
	c.session = fresh
	c.mu.Unlock()
	for _, clan := range c.Clans.Values() {
		clan.SessionToken = fresh.Token
	}
	return fresh
}

// validSession is the guard in front of the client's REST wrappers, port of
// MezonClientCore._getValidSession: ErrNotLoggedIn before Login, otherwise a
// session renewed when needed.
func (c *MezonClient) validSession() (*Session, error) {
	if c.apiClient == nil || c.currentSession() == nil {
		return nil, ErrNotLoggedIn
	}
	return c.ensureValidSession(sessionExpiryBuffer, false)
}

// applySessionEndpoints points the REST client and the socket at the session's
// endpoints, port of MezonClientCore._applySessionEndpoints. It reports
// whether either moved.
func (c *MezonClient) applySessionEndpoints(session *Session) bool {
	changed := false
	if next, host, port, useSSL, ok := apiBasePath(session.APIURL); ok && c.apiClient != nil && c.apiClient.basePath() != next {
		c.Host, c.Port, c.UseSSL = host, port, useSSL
		c.apiClient.SetBasePath(next)
		changed = true
	}
	if c.socket != nil && c.socket.UpdateEndpoint(session.WsURL, c.Host, c.Port, c.UseSSL, session.TcpURL) {
		changed = true
	}
	return changed
}

// resolveHealthyEndpoint asks the login gateway which node the session should
// use after a failed dial, port of MezonClientCore._resolveHealthyEndpoint.
// It returns the session to dial with, whether an endpoint moved, and whether
// the session was renewed because the gateway rejected it (401/403). Lookups
// are rate-limited; a skipped or fruitless one returns the input unchanged.
func (c *MezonClient) resolveHealthyEndpoint(session *Session) (next *Session, endpointChanged, renewed bool, err error) {
	c.mu.Lock()
	if !c.lastHealthyEndpointAsk.IsZero() && time.Since(c.lastHealthyEndpointAsk) < healthyEndpointMinInterval {
		c.mu.Unlock()
		return session, false, false, nil
	}
	c.lastHealthyEndpointAsk = time.Now()
	c.mu.Unlock()

	credential := strings.TrimSpace(session.SessionID)
	if credential == "" {
		credential = strings.TrimSpace(session.Token)
	}
	if credential == "" || c.loginBasePath == "" {
		return session, false, false, nil
	}

	gateway := NewMezonApi(c.Token, c.loginBasePath, c.Timeout)
	payload, status, _ := gateway.GetHealthyEndpoint(credential)
	if payload != nil {
		c.mu.Lock()
		if c.session != nil {
			session = c.session
		}
		patched := *session
		if v := strings.TrimSpace(payload.APIURL); v != "" {
			patched.APIURL = v
		}
		if v := strings.TrimSpace(payload.WsURL); v != "" {
			patched.WsURL = v
		}
		if v := strings.TrimSpace(payload.TcpURL); v != "" {
			patched.TcpURL = v
		}
		if v := strings.TrimSpace(payload.SessionID); v != "" {
			patched.SessionID = v
		}
		c.session = &patched
		c.mu.Unlock()
		return &patched, c.applySessionEndpoints(&patched), false, nil
	}
	if status == 401 || status == 403 {
		fresh, err := c.ensureValidSession(sessionExpiryBuffer, true)
		if err != nil {
			return nil, false, false, err
		}
		return fresh, c.applySessionEndpoints(fresh), true, nil
	}
	return session, false, false, nil
}

// postConnectInit rebuilds clan/channel state after the socket (re)connects:
// join clans and load their channels, then discover DM channels and pre-cache
// them as live TextChannels. Shared by Login and retryConnect, port of the
// common tail of MezonClientCore.handleClientLogin/handleReconnectSocket.
// A DM-channel init failure is non-fatal, mirroring the TS which logs and
// continues.
func (c *MezonClient) postConnectInit(sessionToken string) error {
	if err := c.connectSocket(sessionToken); err != nil {
		return err
	}
	if err := c.channelManager.InitAllDMChannels(sessionToken); err != nil {
		log.Printf("mezon: InitAllDMChannels failed: %v", err)
	}
	c.initDmChannelCache()
	// Join the DM pseudo-clan "0" only now that the DM channels are fetched and
	// cached, port of SocketManager.joinDmClanChat (mezon-sdk v2.8.65). It is
	// sent with is_last_field so its otherwise empty ClanJoin never ends the
	// abridged frame on a 0x00 byte (mezon-sdk v2.16).
	if _, err := c.socket.JoinClanChat("0", true); err != nil {
		return fmt.Errorf("join DM clan chat failed: %w", err)
	}
	return nil
}

// connectSocket joins clan chats and builds Clan objects, port of
// socket_manager.connectSocket / runClanInitPipeline: channels are pre-loaded
// per clan before joining its chat, and failed clans are retried.
func (c *MezonClient) connectSocket(sessionToken string) error {
	clans, err := c.apiClient.ListClanDescs(sessionToken, 0, 0, "")
	if err != nil {
		return fmt.Errorf("listClanDescs failed: %w", err)
	}
	type clanInfo struct {
		id, name, welcome string
	}
	// The global / DM pseudo-clan "0" goes first, port of fetchClanList.
	list := []clanInfo{{"0", "", ""}}
	for _, cl := range clans.GetClandesc() {
		list = append(list, clanInfo{itoaID(cl.ClanId), cl.ClanName, itoaID(cl.WelcomeChannelId)})
	}
	// Give the server a beat after connect before joining clan chats.
	time.Sleep(time.Second)
	// Build every Clan object up front, port of ensureClanObjects. A clan that
	// already exists (reconnect) is refreshed in place instead of recreated so
	// its channel/message caches survive, port of the socket_manager reconnect
	// fix in mezon-js PR #1129.
	for _, cl := range list {
		if existing, ok := c.Clans.Get(cl.id); ok {
			existing.updateFromDesc(cl.name, cl.welcome, sessionToken)
			continue
		}
		clanObj := newClan(cl.id, orString(cl.name, "unknown"), cl.welcome, cl.name, c, sessionToken)
		c.Clans.Set(cl.id, clanObj)
	}
	// Load each clan's channels, then join its chat, port of
	// initClanChannelsAndJoin. The pseudo-clan "0" has no channels to load and
	// is joined later, by postConnectInit, once the DM channels are cached.
	// ReloadChannels (not LoadChannels) so a reconnect re-fetches the list.
	initClan := func(cl clanInfo) bool {
		if cl.id == "0" {
			return true
		}
		clanObj, ok := c.Clans.Get(cl.id)
		if !ok {
			return false
		}
		if err := clanObj.ReloadChannels(); err != nil {
			return false
		}
		time.Sleep(50 * time.Millisecond)
		_, err := c.socket.JoinClanChat(cl.id, false)
		return err == nil
	}
	var failed []clanInfo
	for _, cl := range list {
		if !initClan(cl) {
			failed = append(failed, cl)
		}
	}
	// Up to 5 retry rounds, pacing harder each attempt. The TS widens its
	// global API rate-limiter delay by 500ms per attempt; without a limiter we
	// sleep the same amount before each retried clan.
	for attempt := 1; attempt <= 5 && len(failed) > 0; attempt++ {
		var still []clanInfo
		for _, cl := range failed {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			if !initClan(cl) {
				still = append(still, cl)
			}
		}
		failed = still
	}
	// TS leaves clans that never initialized silently uninitialized; log so a
	// stuck clan is at least visible.
	for _, cl := range failed {
		log.Printf("mezon: init clan %s failed after retries", cl.id)
	}
	return nil
}

// retryConnect re-establishes the connection after a disconnect, retrying with
// exponential backoff until it succeeds or the client is closed, port of
// SocketManager.retriesConnect.
func (c *MezonClient) retryConnect() {
	c.mu.Lock()
	if c.reconnecting || c.hardDisconnect {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	delay := reconnectInitialDelay
	for {
		time.Sleep(delay)
		if c.isHardDisconnected() {
			break
		}
		if err := c.handleReconnect(); err != nil {
			if c.isHardDisconnected() {
				break
			}
			if delay = delay * 2; delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			log.Printf("mezon: reconnect failed, retrying in %s: %v", delay, err)
			continue
		}
		// Leave only when the connection is still up (or the client was
		// closed): a drop during the init above fired OnDisconnect while
		// reconnecting was set, so nobody else is going to retry it.
		c.mu.Lock()
		if c.hardDisconnect || c.socket.IsOpen() {
			c.reconnecting = false
			c.mu.Unlock()
			log.Printf("mezon: reconnected successfully")
			return
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.reconnecting = false
	c.mu.Unlock()
}

// handleReconnect is one reconnect attempt: authenticate again, dial, and when
// the dial fails ask the gateway for a healthy node and dial that. The dial and
// healthy-node steps are a port of MezonClientCore.handleReconnectSocket.
//
// Every attempt starts from a fresh session, where the TS only renews one that
// has expired by the clock: a token the server revoked early is rejected just
// the same, the healthy-node lookup cannot be relied on to say so, and the new
// session also carries the endpoints currently assigned to the bot. When the
// gateway cannot be reached, a session that has not expired is still tried.
//
// Unlike the TS, a failed re-authentication is not followed by a full login():
// that would authenticate with the same credentials against the same gateway,
// so the attempt simply fails and the backoff loop retries it.
func (c *MezonClient) handleReconnect() error {
	session, err := c.ensureValidSession(sessionExpiryBuffer, true)
	if err != nil {
		session = c.currentSession()
		if session == nil || session.IsExpired(time.Now().Add(sessionExpiryBuffer).Unix()) {
			return err
		}
		log.Printf("mezon: reconnect could not renew the session, trying the current one: %v", err)
	}
	c.applySessionEndpoints(session)

	if err := c.socket.Connect(session, true); err != nil {
		resolved, endpointChanged, renewed, rerr := c.resolveHealthyEndpoint(session)
		if rerr != nil {
			return rerr
		}
		if !endpointChanged && !renewed {
			return err
		}
		session = resolved
		if err := c.socket.Connect(session, true); err != nil {
			return err
		}
	}
	if err := c.postConnectInit(session.Token); err != nil {
		// Drop the connection so the next attempt dials again with a fresh
		// session instead of reusing this half-initialized one.
		c.socket.Close()
		return err
	}
	return nil
}

// Close shuts down the socket and prevents reconnection.
func (c *MezonClient) Close() {
	c.mu.Lock()
	c.hardDisconnect = true
	c.mu.Unlock()
	if c.socket != nil {
		c.socket.Close()
	}
}

// Socket returns the underlying socket for advanced/raw realtime calls.
func (c *MezonClient) Socket() *DefaultSocket { return c.socket }

// Api returns the REST client bound to this MezonClient (nil before Login).
// Calls need a session token; see EnsureValidSession.
func (c *MezonClient) Api() *MezonApi { return c.apiClient }

// SessionToken returns the current session's bearer token, or "" before Login.
// The token may be expired; EnsureValidSession renews it first.
func (c *MezonClient) SessionToken() string {
	if session := c.currentSession(); session != nil {
		return session.Token
	}
	return ""
}

// CreatePoll creates a poll in a channel, port of MezonClientCore.createPoll.
func (c *MezonClient) CreatePoll(req *api.CreatePollRequest) (*api.CreatePollResponse, error) {
	session, err := c.validSession()
	if err != nil {
		return nil, err
	}
	return c.apiClient.CreatePoll(session.Token, req)
}

// ClosePoll closes a poll, port of MezonClientCore.closePoll. Pass pollID, or
// messageID + channelID with an empty pollID.
func (c *MezonClient) ClosePoll(pollID, messageID, channelID string) error {
	session, err := c.validSession()
	if err != nil {
		return err
	}
	return c.apiClient.ClosePoll(session.Token, pollID, messageID, channelID)
}

// GetPoll fetches a poll with results, port of MezonClientCore.getPoll. Pass
// pollID, or messageID + channelID with an empty pollID.
func (c *MezonClient) GetPoll(pollID, messageID, channelID string) (*api.GetPollResponse, error) {
	session, err := c.validSession()
	if err != nil {
		return nil, err
	}
	return c.apiClient.GetPoll(session.Token, pollID, messageID, channelID)
}

// GenerateMeetToken issues an SFU access token for a voice channel, port of
// MezonClientCore.generateMeetToken. See MezonApi.GenerateMeetToken for the
// optional req.CurrentEndpointId.
func (c *MezonClient) GenerateMeetToken(req *api.GenerateMeetTokenRequest) (*api.GenerateMeetTokenResponse, error) {
	session, err := c.validSession()
	if err != nil {
		return nil, err
	}
	return c.apiClient.GenerateMeetToken(session.Token, req)
}

// GetStreamPlayback returns where to publish to or watch a stream channel,
// port of MezonClientCore.getStreamPlayback. With req.Publish the response
// carries the WHIP URL and bearer token a bot needs to go live.
func (c *MezonClient) GetStreamPlayback(req *api.GetStreamPlaybackRequest) (*api.GetStreamPlaybackResponse, error) {
	session, err := c.validSession()
	if err != nil {
		return nil, err
	}
	return c.apiClient.GetStreamPlayback(session.Token, req)
}

// GenerateCDNSignature generates a CDN signature for a channel, port of
// MezonClientCore.generateCDNSignature.
func (c *MezonClient) GenerateCDNSignature(req *api.GenerateCDNSignatureRequest) (*api.GenerateCDNSignatureResponse, error) {
	session, err := c.validSession()
	if err != nil {
		return nil, err
	}
	return c.apiClient.GenerateCDNSignature(session.Token, req)
}

// ChannelManager returns the DM channel manager.
func (c *MezonClient) ChannelManager() *ChannelManager { return c.channelManager }

func orString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
