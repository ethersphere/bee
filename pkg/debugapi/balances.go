package debugapi

import (
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"

	"net/http"
)

type balanceResponse struct {
	Peer    string `json:"peer"`
	Balance int64  `json:"balance"`
}

type balancesResponse struct {
	Balances []balanceResponse `json: "balances"`
}

func (s *server) balancesHandler(w http.ResponseWriter, r *http.Request) {

	balances, err := s.Accounting.Balances()

	if err != nil {
		s.Logger.Debugf("debug api: balances: %v", err)
	}

	var balResponses []balanceResponse

	for key, value := range balances {
		balResponses = append(balResponses, balanceResponse{
			Peer:    key,
			Balance: value,
		})
	}

	jsonhttp.OK(w, balResponses)

}

func (s *server) balancesPeerHandler(w http.ResponseWriter, r *http.Request) {

	peer, err := swarm.ParseHexAddress(mux.Vars(r)["peer"])
	if err != nil {
		s.Logger.Debugf("debug api: balances peer: parse peer address: %v", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	balance, err := s.Accounting.Balance(peer)

	if err != nil {
		s.Logger.Debugf("debug-api: balances peer: get peer balance: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: balance,
	})

	//

}
