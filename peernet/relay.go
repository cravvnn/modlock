package peernet

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// RelayPath is where the service issues relay credentials to a signed-in
// creator.
const RelayPath = "/api/turn-credentials"

// maxRelayResponse bounds the credentials response.
const maxRelayResponse = 64 << 10

// directServer finds each peer's public address for a direct link. It is the
// transport's own default, which the relay's configuration replaces.
const directServer = "stun:stun.l.google.com:19302"

// Relay supplies relay credentials from the creator's sign-in to one local
// peer's links, for the PCs that cannot link directly. Without them a link
// still tries the direct path.
type Relay struct {
	// le logs why a link goes without the relay.
	le *logrus.Entry
	// local is the peer whose links use the relay.
	local peer.ID
	// origin is the service that issues the credentials.
	origin string
	// signIn reads the saved sign-in.
	signIn func() SignIn
}

// NewRelay constructs the relay credentials for local's links.
func NewRelay(le *logrus.Entry, local peer.ID, origin string, signIn func() SignIn) *Relay {
	return &Relay{le: le, local: local, origin: origin, signIn: signIn}
}

// GetControllerInfo identifies the relay credentials.
func (r *Relay) GetControllerInfo() *controller.Info {
	return controller.NewInfo("modlock/peernet/relay", controller.MustParseVersion("0.0.1"), "relay credentials from the sign-in")
}

// Execute does nothing; credentials are fetched for each connection attempt.
func (r *Relay) Execute(context.Context) error { return nil }

// Close does nothing.
func (r *Relay) Close() error { return nil }

// HandleDirective supplies the credentials to the local peer only.
func (r *Relay) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(webrtc.LookupICEProvider)
	if !ok || d.ICEPeerID() != r.local {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]webrtc.ICEProvider{r})), nil
}

// ICEConfig returns the direct path and fresh relay credentials. When the
// credentials are unavailable, such as while signed out, the link tries the
// direct path alone.
func (r *Relay) ICEConfig(ctx context.Context) (*webrtc.WebRtcConfig, error) {
	// Offer the direct path first, then whichever relays the sign-in allows.
	config := &webrtc.WebRtcConfig{IceServers: []*webrtc.IceServerConfig{{Urls: []string{directServer}}}}
	relays, err := r.fetch(ctx)
	if err != nil {
		r.le.WithError(err).Debug("linking without the relay")
	}
	config.IceServers = append(config.IceServers, relays...)
	return config, nil
}

// fetch asks the service for relay credentials with the saved sign-in.
func (r *Relay) fetch(ctx context.Context) ([]*webrtc.IceServerConfig, error) {
	// Ask the service with the sign-in.
	in := r.signIn()
	if in.Token == "" {
		return nil, errors.New("signed out")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.origin+RelayPath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+in.Token)

	// Accept only a successful answer.
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "fetch relay credentials")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("the relay service answered HTTP %d", response.StatusCode)
	}

	// Read the credentials and keep the TURN servers that carry them.
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRelayResponse))
	if err != nil {
		return nil, errors.Wrap(err, "read relay credentials")
	}
	credentials := &RelayCredentials{}
	if err := credentials.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "read relay credentials")
	}
	return relayServers(credentials, time.Now())
}

// relayServers narrows credentials to the authenticated TURN servers, since
// STUN alone cannot relay a link.
func relayServers(credentials *RelayCredentials, now time.Time) ([]*webrtc.IceServerConfig, error) {
	// Refuse credentials about to expire.
	if credentials.GetExpiresAt() <= now.Add(time.Minute).UnixMilli() {
		return nil, errors.New("the relay credentials have expired")
	}

	// Keep each server's TURN addresses.
	var servers []*webrtc.IceServerConfig
	for _, server := range credentials.GetServers() {
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
	if len(servers) == 0 {
		return nil, errors.New("the relay service returned no usable servers")
	}
	return servers, nil
}

// _ is a type assertion.
var (
	_ controller.Controller = (*Relay)(nil)
	_ webrtc.ICEProvider    = (*Relay)(nil)
)
