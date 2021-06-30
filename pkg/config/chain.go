package config

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	// chain ID
	goerliChainID = int64(5)
	xdaiChainID   = int64(100)
	// start block
	GoerliStartBlock = uint64(4933174)
	xdaiStartBlock   = uint64(16515648)
	// factory address
	goerliContractAddress      = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
	xdaiContractAddress        = common.HexToAddress("0x0FDc5429C50e2a39066D8A94F3e2D2476fcc3b85")
	goerliFactoryAddress       = common.HexToAddress("0x73c412512E1cA0be3b89b77aB3466dA6A1B9d273")
	goerliLegacyFactoryAddress = common.HexToAddress("0xf0277caffea72734853b834afc9892461ea18474")
	xdaiFactoryAddress         = common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420")
	// postage stamp
	goerliPostageStampContractAddress = common.HexToAddress("0x621e455C4a139f5C4e4A8122Ce55Dc21630769E4")
	xdaiPostageStampContractAddress   = common.HexToAddress("0x6a1a21eca3ab28be85c7ba22b2d6eae5907c900e")
)

type ChainConfig struct {
	StartBlock         uint64
	LegacyFactories    []common.Address
	PostageStamp       common.Address
	CurrentFactory     common.Address
	PriceOracleAddress common.Address
}

func GetChainConfig(chainID int64) (cfg ChainConfig, found bool) {
	postageStamp, startBlock, found := discoverAddresses(chainID)
	if !found {
		return ChainConfig{}, false
	}

	cfg.PostageStamp = postageStamp
	cfg.StartBlock = startBlock

	currentFactory, legacyFactories, found := discoverFactoryAddress(chainID)
	if !found {
		return ChainConfig{}, false
	}

	cfg.CurrentFactory = currentFactory
	cfg.LegacyFactories = legacyFactories

	priceOracleAddress, found := discoverPriceOracleAddress(chainID)
	if !found {
		return ChainConfig{}, false
	}

	cfg.PriceOracleAddress = priceOracleAddress

	return
}

// DiscoverAddresses returns the canonical contracts for this chainID
func discoverAddresses(chainID int64) (postageStamp common.Address, startBlock uint64, found bool) {
	switch chainID {
	case goerliChainID:
		return goerliPostageStampContractAddress, GoerliStartBlock, true
	case xdaiChainID:
		return xdaiPostageStampContractAddress, xdaiStartBlock, true
	default:
		return common.Address{}, 0, false
	}
}

// DiscoverFactoryAddress returns the canonical factory for this chainID
func discoverFactoryAddress(chainID int64) (currentFactory common.Address, legacyFactories []common.Address, found bool) {
	switch chainID {
	case goerliChainID:
		// goerli
		return goerliFactoryAddress, []common.Address{
			goerliLegacyFactoryAddress,
		}, true
	case xdaiChainID:
		// xdai
		return xdaiFactoryAddress, []common.Address{}, true
	default:
		return common.Address{}, nil, false
	}
}

// DiscoverPriceOracleAddress returns the canonical price oracle for this chainID
func discoverPriceOracleAddress(chainID int64) (priceOracleAddress common.Address, found bool) {
	switch chainID {
	case goerliChainID:
		return goerliContractAddress, true
	case xdaiChainID:
		return xdaiContractAddress, true
	}
	return common.Address{}, false
}
