package peernet

import (
	"context"
	"time"

	"github.com/aperturerobotics/go-websocket"
	"github.com/paralin/modlock/publish"
	"github.com/pkg/errors"
)

// DevLobbyPath is where the service keeps a hosted session's dev lobby open
// for as long as its socket stays open.
const DevLobbyPath = "/api/dev-lobby"

// lobbyAnswerTimeout bounds opening the dev lobby socket and reading the
// service's answer.
const lobbyAnswerTimeout = 30 * time.Second

// holdLobby keeps a dev lobby for the hosted session secret open at the
// service until ctx ends, reporting each change. It opens the socket again
// after it drops, which resumes the lobby, and stops once the service refuses
// or there is no sign-in. Ending ctx closes the socket normally, which ends
// the lobby.
func (s *Service) holdLobby(ctx context.Context, req *LobbyRequest, secret uint64, report func(*LobbyState)) {
	open := &publish.DevLobbyOpen{ModName: req.GetModName(), Version: req.GetVersion(), Secret: secret}
	retry := time.Second
	for {
		// Hold the lobby with the current sign-in.
		token := s.signIn().Token
		if token == "" {
			report(&LobbyState{State: &LobbyState_SignedOut{SignedOut: true}})
			return
		}
		err := s.holdLobbySocket(ctx, token, open, func(state *publish.DevLobbyState) {
			retry = time.Second
			report(lobbyState(state))
		})
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, errRefused):
			report(&LobbyState{State: &LobbyState_Refused{Refused: "no lobby: " + s.origin + " refused the saved sign-in; modlock login signs in again"}})
			return
		case errors.Is(err, errLobbyRefused):
			return
		}

		// Open it again after a growing wait.
		report(&LobbyState{State: &LobbyState_Reconnecting{Reconnecting: err.Error()}})
		select {
		case <-time.After(retry):
		case <-ctx.Done():
			return
		}
		retry = min(retry*2, admitRetryMax)
	}
}

// errLobbyRefused reports that the service answered with a refusal.
var errLobbyRefused = errors.New("the service refused the lobby")

// holdLobbySocket opens one dev lobby socket with token, passes the service's
// answer to answered, and holds the socket until it drops or ctx ends. It
// returns errLobbyRefused after a refusal.
func (s *Service) holdLobbySocket(ctx context.Context, token string, open *publish.DevLobbyOpen, answered func(*publish.DevLobbyState)) error {
	// Open the socket.
	dial, cancel := context.WithTimeout(ctx, lobbyAnswerTimeout)
	defer cancel()
	conn, err := dialSigned(dial, s.origin+DevLobbyPath, token, s.key)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	// Send the request. Reading on a context that ends would drop the socket
	// without a close, which only marks the host gone, so the socket reads
	// until it closes and ctx ending closes it normally.
	answer, cancelAnswer := context.WithTimeout(context.WithoutCancel(ctx), lobbyAnswerTimeout)
	defer cancelAnswer()
	data, err := open.MarshalVT()
	if err == nil {
		err = conn.Write(answer, websocket.MessageBinary, data)
	}
	if err != nil {
		return err
	}

	// Read the answer.
	_, data, err = conn.Read(answer)
	if err != nil {
		return err
	}
	state := &publish.DevLobbyState{}
	if err := state.UnmarshalVT(data); err != nil {
		return err
	}
	answered(state)
	if state.GetRefused() != nil {
		return errLobbyRefused
	}

	// Ping it until either side closes it.
	closed := conn.CloseRead(context.WithoutCancel(ctx))
	ticker := time.NewTicker(admitPing)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return conn.Close(websocket.StatusNormalClosure, "")
		case <-closed.Done():
			return errors.New("the connection to " + s.origin + " was lost")
		case <-ticker.C:
			if err := conn.Ping(closed); err != nil {
				return err
			}
		}
	}
}

// lobbyState reports the service's answer as a lobby state.
func lobbyState(state *publish.DevLobbyState) *LobbyState {
	if refused := state.GetRefused(); refused != nil {
		return &LobbyState{State: &LobbyState_Refused{Refused: refused.GetMessage()}}
	}
	return &LobbyState{State: &LobbyState_Url{Url: state.GetUrl()}}
}
