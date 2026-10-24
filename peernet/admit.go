package peernet

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// AdmitPath is where the service admits a signed-in peer to signaling for as
// long as its socket stays open.
const AdmitPath = "/api/signal/peer"

// admitPing is how often an open admission socket proves it is alive, inside
// the idle timeout of the proxies between the PC and the service.
const admitPing = 30 * time.Second

// admitRetryMax bounds the wait between attempts after a socket drops.
const admitRetryMax = 30 * time.Second

// errRefused reports that the service refused the sign-in.
var errRefused = errors.New("the service refused the sign-in")

// SignIn is the creator's saved sign-in with the service. An empty Token
// means signed out.
type SignIn struct {
	// Token is the bearer credential.
	Token string
	// Account is the signed-in name.
	Account string
}

// Admission holds a peer's admission to signaling at a service. It reads the
// sign-in again before each attempt, retries a dropped socket, and waits for
// Wake while signed out or refused, since only a new sign-in can change that.
type Admission struct {
	// origin is the service, such as https://hyperline.gg.
	origin string
	// key is the peer's key, which signs each socket.
	key crypto.PrivKey
	// signIn reads the saved sign-in.
	signIn func() SignIn
	// wake asks a waiting Run to read the sign-in again.
	wake chan struct{}

	// mtx guards the fields below.
	mtx sync.Mutex
	// state is where the peer stands with the service.
	state AdmissionState
	// account is the signed-in name of the current attempt.
	account string
	// err says why the last attempt failed, while connecting.
	err string
}

// NewAdmission constructs the admission of key at origin.
func NewAdmission(origin string, key crypto.PrivKey, signIn func() SignIn) *Admission {
	return &Admission{origin: origin, key: key, signIn: signIn, wake: make(chan struct{}, 1)}
}

// Wake asks Run to read the sign-in again, such as after a client signed in.
func (a *Admission) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Snapshot returns the admission's state, signed-in name and last failure.
func (a *Admission) Snapshot() (AdmissionState, string, string) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.state, a.account, a.err
}

// set records the admission's state.
func (a *Admission) set(state AdmissionState, account, err string) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.state, a.account, a.err = state, account, err
}

// Run holds the admission until ctx ends.
func (a *Admission) Run(ctx context.Context) {
	retry := time.Second
	var refused string
	for {
		// Without a usable sign-in, wait for a new one.
		in := a.signIn()
		if in.Token == "" || in.Token == refused {
			if in.Token == "" {
				a.set(AdmissionState_ADMISSION_STATE_SIGNED_OUT, "", "")
			}
			select {
			case <-a.wake:
				continue
			case <-ctx.Done():
				return
			}
		}

		// Hold the socket until it drops or the saved sign-in changes.
		a.set(AdmissionState_ADMISSION_STATE_CONNECTING, in.Account, "")
		changed, err := a.holdUntilChanged(ctx, in.Token, func() {
			a.set(AdmissionState_ADMISSION_STATE_ADMITTED, in.Account, "")
			retry = time.Second
		})
		if ctx.Err() != nil {
			return
		}
		if changed {
			continue
		}
		if errors.Is(err, errRefused) {
			refused = in.Token
			a.set(AdmissionState_ADMISSION_STATE_REFUSED, in.Account, "")
			continue
		}

		// Retry a dropped socket after a growing wait.
		a.set(AdmissionState_ADMISSION_STATE_CONNECTING, in.Account, err.Error())
		select {
		case <-time.After(retry):
		case <-a.wake:
		case <-ctx.Done():
			return
		}
		retry = min(retry*2, admitRetryMax)
	}
}

// holdUntilChanged holds the admission socket with token, and reports changed
// when a Wake finds a different saved sign-in, such as after a sign-out, so
// Run attempts again at once with the new one.
func (a *Admission) holdUntilChanged(ctx context.Context, token string, admitted func()) (bool, error) {
	// Cancel the socket when a Wake reads a different sign-in.
	holdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watched := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-a.wake:
				if a.signIn().Token != token {
					watched <- true
					cancel()
					return
				}
			case <-holdCtx.Done():
				watched <- false
				return
			}
		}
	}()

	// Hold the socket, then wait for the watch to stop reading wakes.
	err := hold(holdCtx, a.origin, token, a.key, admitted)
	cancel()
	return <-watched, err
}

// hold opens key's admission socket at origin with the bearer token, calls
// admitted once it is open, and pings it until it drops or ctx ends.
func hold(ctx context.Context, origin, token string, key crypto.PrivKey, admitted func()) error {
	// Sign the socket's request with the peer's key and carry the sign-in.
	pid, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+AdmitPath, nil)
	if err != nil {
		return err
	}
	if err := transport.SignHTTPRequest(request, nil, key, pid, SigningEnv); err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)

	// Open the socket; the service answers 401 to a sign-in it refuses.
	conn, response, err := websocket.Dial(ctx, origin+AdmitPath, &websocket.DialOptions{HTTPHeader: request.Header})
	if err != nil {
		if response != nil && response.StatusCode == http.StatusUnauthorized {
			return errRefused
		}
		return err
	}
	defer conn.CloseNow()
	admitted()

	// Ping it until either side closes it.
	closed := conn.CloseRead(ctx)
	ticker := time.NewTicker(admitPing)
	defer ticker.Stop()
	for {
		select {
		case <-closed.Done():
			return errors.New("the service closed the admission socket")
		case <-ticker.C:
			if err := conn.Ping(closed); err != nil {
				return err
			}
		}
	}
}
