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

func (s *server) balancesHandler(w http.ResponseWriter, r *http.Request) {

	/*
		ms, ok := s.balancesDriver.(json.Marshaler)
		if !ok {
			s.Logger.Error("balances driver cast to json marshaler")
			jsonhttp.InternalServerError(w, "balances json marshal interface error")
			return
		}

		b, err := ms.MarshalJSON()
		if err != nil {
			s.Logger.Errorf("balances marshal to json: %v", err)
			jsonhttp.InternalServerError(w, err)
			return
		}
		w.Header().Set("Content-Type", jsonhttp.DefaultContentTypeHeader)
		_, _ = io.Copy(w, bytes.NewBuffer(b))
	*/
}

func (s *server) balancesPeerHandler(w http.ResponseWriter, r *http.Request) {

	peer, err := swarm.ParseHexAddress(mux.Vars(r)["peer"])
	if err != nil {
		s.Logger.Debugf("debug api: balances peer: parse peer address: %v", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}
	//
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

}
