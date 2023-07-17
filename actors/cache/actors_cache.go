package cache

import (
	"github.com/filecoin-project/go-address"
	filTypes "github.com/filecoin-project/lotus/chain/types"
	"github.com/zondax/fil-parser/actors/cache/impl"
	"github.com/zondax/fil-parser/actors/cache/impl/common"
	"github.com/zondax/fil-parser/types"
	"go.uber.org/zap"
)

// SystemActorsId Map to identify system actors which don't have an associated robust address
var SystemActorsId = map[string]bool{
	"f00":  true,
	"f01":  true,
	"f02":  true,
	"f03":  true,
	"f04":  true,
	"f05":  true,
	"f06":  true,
	"f07":  true,
	"f099": true,
}

func SetupActorsCache(dataSource common.DataSource) (*ActorsCache, error) {
	var offlineCache IActorsCache
	var onChainCache impl.OnChain

	err := onChainCache.NewImpl(dataSource)
	if err != nil {
		return nil, err
	}

	// Try kvStore cache, if it fails, on-memory cache
	var kvStoreCache impl.KVStore
	err = kvStoreCache.NewImpl(dataSource)
	if err == nil {
		offlineCache = &kvStoreCache
	} else {
		zap.S().Warn("[ActorsCache] - Unable to initialize kv store cache. Using on-memory cache")
		var inMemoryCache impl.Memory
		err = inMemoryCache.NewImpl(dataSource)
		if err != nil {
			zap.S().Errorf("[ActorsCache] - Unable to initialize on-memory cache: %s", err.Error())
			return nil, err
		}
		offlineCache = &inMemoryCache
	}

	zap.S().Infof("[ActorsCache] - Actors cache initialized. Offline cache implementation: %s", offlineCache.ImplementationType())

	return &ActorsCache{
		offlineCache: offlineCache,
		onChainCache: &onChainCache,
	}, nil
}

func (a *ActorsCache) GetActorCode(add address.Address, key filTypes.TipSetKey) (string, error) {
	// Try kv store cache
	actorCode, err := a.offlineCache.GetActorCode(add, key)
	if err == nil {
		return actorCode, nil
	}

	zap.S().Debugf("[ActorsCache] - Unable to retrieve actor code from kv store for address %s. Trying on-chain cache", add.String())
	// Try on-chain cache
	actorCode, err = a.onChainCache.GetActorCode(add, key)
	if err != nil {
		zap.S().Error("[ActorsCache] - Unable to retrieve actor code from node: %s", err.Error())
		return "", err
	}

	// Code is not cached, store it
	err = a.storeActorCode(add, types.AddressInfo{
		ActorCid: actorCode,
	})

	if err != nil {
		zap.S().Errorf("[ActorsCache] - Unable to store address info: %s", err.Error())
		return "", err
	}

	return actorCode, nil
}

func (a *ActorsCache) GetRobustAddress(add address.Address) (string, error) {
	if _, ok := SystemActorsId[add.String()]; ok {
		return add.String(), nil
	}

	// Try kv store cache
	robust, err := a.offlineCache.GetRobustAddress(add)
	if err == nil {
		return robust, nil
	}

	zap.S().Debugf("[ActorsCache] - Unable to retrieve robust address from kv store for address %s. Trying on-chain cache", add.String())

	// Try on-chain cache
	robust, err = a.onChainCache.GetRobustAddress(add)
	if err != nil {
		zap.S().Errorf("[ActorsCache] - Unable to retrieve actor code from node: %s", err.Error())
		return "", err
	}

	// Robust address is not cached, store it
	err = a.storeRobustAddress(add, types.AddressInfo{
		Robust: robust,
	})

	if err != nil {
		zap.S().Errorf("[ActorsCache] - Unable to store address info: %s", err.Error())
		return "", err
	}

	return robust, nil
}

func (a *ActorsCache) GetShortAddress(add address.Address) (string, error) {
	// Try kv store cache
	short, err := a.offlineCache.GetShortAddress(add)
	if err == nil {
		return short, nil
	}

	zap.S().Debugf("[ActorsCache] - Unable to retrieve short address from kv store for address %s. Trying on-chain cache", add.String())

	// Try on-chain cache
	short, err = a.onChainCache.GetShortAddress(add)
	if err != nil {
		zap.S().Error("[ActorsCache] - Unable to retrieve actor code from node: %s", err.Error())
		return "", err
	}

	// Robust address is not cached, store it
	err = a.storeShortAddress(add, types.AddressInfo{
		Short: short,
	})

	if err != nil {
		zap.S().Errorf("[ActorsCache] - Unable to store address info: %s", err.Error())
		return "", err
	}

	return short, nil
}

func (a *ActorsCache) storeActorCode(add address.Address, info types.AddressInfo) error {
	shortAddress, err := a.GetShortAddress(add)
	if err != nil {
		return err
	}

	a.offlineCache.StoreAddressInfo(types.AddressInfo{
		Short:    shortAddress,
		ActorCid: info.ActorCid,
	})

	return nil
}

func (a *ActorsCache) storeShortAddress(add address.Address, info types.AddressInfo) error {
	robustAddress, err := a.GetRobustAddress(add)
	if err != nil {
		return err
	}

	a.offlineCache.StoreAddressInfo(types.AddressInfo{
		Short:  info.Short,
		Robust: robustAddress,
	})

	return nil
}

func (a *ActorsCache) storeRobustAddress(add address.Address, info types.AddressInfo) error {
	shortAddress, err := a.GetShortAddress(add)
	if err != nil {
		return err
	}

	a.offlineCache.StoreAddressInfo(types.AddressInfo{
		Short:  shortAddress,
		Robust: info.Robust,
	})

	return nil
}
