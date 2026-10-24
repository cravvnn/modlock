package peernet

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/paralin/modlock/publish"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
)

// directServer finds each peer's public address for a direct link. It is the
// transport's own default, which the relay's configuration replaces.
const directServer = "stun:stun.l.google.com:19302"

// Relay offers the service's TURN servers to one local peer's new links while
// the peer hosts a dev lobby, for the PCs that cannot link directly. One relay
// on the host's side is enough for a guest behind any router. Without a lobby
// a link tries the direct path alone.
type Relay struct {
	// local is the peer whose links use the relay.
	local peer.ID
	// current holds the hosted lobby's relay, or nil without one.
	current atomic.Pointer[publish.Relay]
}

// NewRelay constructs the relay for local's links, holding no servers.
func NewRelay(local peer.ID) *Relay {
	return &Relay{local: local}
}

// Hold sets the relay new links use; nil drops it.
func (r *Relay) Hold(relay *publish.Relay) {
	r.current.Store(relay)
}

// GetControllerInfo identifies the relay.
func (r *Relay) GetControllerInfo() *controller.Info {
	return controller.NewInfo("modlock/peernet/relay", controller.MustParseVersion("0.0.1"), "dev lobby relay servers")
}

// Execute does nothing; each new link reads the relay it holds then.
func (r *Relay) Execute(context.Context) error { return nil }

// Close does nothing.
func (r *Relay) Close() error { return nil }

// HandleDirective supplies the configuration to the local peer only.
func (r *Relay) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(webrtc.LookupICEProvider)
	if !ok || d.ICEPeerID() != r.local {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]webrtc.ICEProvider{r})), nil
}

// ICEConfig offers the direct path first, then the relay's TURN servers
// while their credentials last.
func (r *Relay) ICEConfig(context.Context) (*webrtc.WebRtcConfig, error) {
	config := &webrtc.WebRtcConfig{IceServers: []*webrtc.IceServerConfig{{Urls: []string{directServer}}}}
	config.IceServers = append(config.IceServers, relayServers(r.current.Load(), time.Now())...)
	return config, nil
}

// relayServers narrows relay to its authenticated TURN servers, since STUN
// alone cannot relay a link, or none once its credentials are about to expire.
func relayServers(relay *publish.Relay, now time.Time) []*webrtc.IceServerConfig {
	// Skip credentials about to expire.
	if relay.GetExpiresAt() <= now.Add(time.Minute).UnixMilli() {
		return nil
	}

	// Keep each server's TURN addresses.
	var servers []*webrtc.IceServerConfig
	for _, server := range relay.GetServers() {
		if server.GetUsername() == "" || server.GetCredential() == "" {
			continue
		}
		ice := &webrtc.IceServerConfig{
			Username:   server.GetUsername(),
			Credential: &webrtc.IceServerConfig_Password{Password: server.GetCredential()},
		}
		for _, url := range server.GetUrls() {
			if strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:") {
				ice.Urls = append(ice.Urls, url)
			}
		}
		if len(ice.Urls) != 0 {
			servers = append(servers, ice)
		}
	}
	return servers
}

// _ is a type assertion.
var (
	_ controller.Controller = (*Relay)(nil)
	_ webrtc.ICEProvider    = (*Relay)(nil)
)
