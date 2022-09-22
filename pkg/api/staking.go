package api

import (
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

type stakingResponse struct {
}

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	overlayAddr := mux.Vars(r)["address"]
	stakedAmount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "deposit stake: invalid amount")
		jsonhttp.BadRequest(w, "invalid staking amount")
		return
	}

}

func (s *Service) stakingGetStakedAmount(w http.ResponseWriter, r *http.Request) {
	overlayAddr := mux.Vars(r)["address"]

}