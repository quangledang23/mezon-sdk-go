package mezon

import (
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// ConvertChannelTypeToChannelMode maps a channel type to its stream mode,
// port of convertChanneltypeToChannelMode in src/utils/helper.ts.
func ConvertChannelTypeToChannelMode(channelType int) ChannelStreamMode {
	switch ChannelType(channelType) {
	case ChannelTypeDM:
		return StreamModeDM
	case ChannelTypeGroup:
		return StreamModeGroup
	case ChannelTypeChannel, ChannelTypeApp, ChannelTypeMezonVoice:
		return StreamModeChannel
	case ChannelTypeThread:
		return StreamModeThread
	}
	return 0
}

var numericIDRe = regexp.MustCompile(`^\d+$`)

// IsValidUserID reports whether id is a numeric snowflake id.
func IsValidUserID(id string) bool {
	return id != "" && numericIDRe.MatchString(id)
}

// ParseURLToHostAndSSL splits a URL into host, port and SSL flag,
// port of parseUrlToHostAndSSL in src/utils/helper.ts.
func ParseURLToHostAndSSL(urlStr string) (host, port string, useSSL bool, err error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", "", false, err
	}
	useSSL = u.Scheme == "https"
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		if useSSL {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port, useSSL, nil
}

// snowflake generator, port of generateSnowflakeId in src/utils/helper.ts.
var (
	snowflakeMu   sync.Mutex
	snowflakeSeq  int64
	snowflakeLast int64
)

const snowflakeEpoch = int64(1577836800000)

// GenerateSnowflakeID returns a unique snowflake id as a decimal string.
func GenerateSnowflakeID() string {
	snowflakeMu.Lock()
	defer snowflakeMu.Unlock()

	ts := time.Now().UnixMilli()
	if ts == snowflakeLast {
		snowflakeSeq++
	} else {
		snowflakeSeq = 0
		snowflakeLast = ts
	}
	const workerID = int64(1)
	const dataCenterID = int64(1)
	id := ((ts - snowflakeEpoch) << 22) | (dataCenterID << 17) | (workerID << 12) | snowflakeSeq
	return strconv.FormatInt(id, 10)
}
