package bor

import (
	"errors"
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/maticnetwork/heimdall/bor/types"
	"github.com/maticnetwork/heimdall/helper"
	"github.com/maticnetwork/heimdall/staking"
	hmTypes "github.com/maticnetwork/heimdall/types"
)

var (
	DefaultValue = []byte{0x01} // Value to store in CacheCheckpoint and CacheCheckpointACK & ValidatorSetChange Flag

	SpanDurationKey       = []byte{0x24} // Key to store span duration for Bor
	SprintDurationKey     = []byte{0x25} // Key to store span duration for Bor
	LastSpanIDKey         = []byte{0x35} // Key to store last span start block
	SpanPrefixKey         = []byte{0x36} // prefix key to store span
	SpanCacheKey          = []byte{0x37} // key to store Cache for span
	LastProcessedEthBlock = []byte{0x38} // key to store last processed eth block for seed
)

// Keeper stores all related data
type Keeper struct {
	cdc *codec.Codec
	sk  staking.Keeper
	// The (unexposed) keys used to access the stores from the Context.
	storeKey sdk.StoreKey
	// codespace
	codespace sdk.CodespaceType
	// param space
	paramSpace params.Subspace
	// contract caller
	contractCaller helper.ContractCaller
}

// NewKeeper create new keeper
func NewKeeper(
	cdc *codec.Codec,
	storeKey sdk.StoreKey,
	paramSpace params.Subspace,
	codespace sdk.CodespaceType,
	stakingKeeper staking.Keeper,
	caller helper.ContractCaller,
) Keeper {
	// create keeper
	keeper := Keeper{
		cdc:            cdc,
		storeKey:       storeKey,
		paramSpace:     paramSpace.WithKeyTable(types.ParamKeyTable()),
		codespace:      codespace,
		sk:             stakingKeeper,
		contractCaller: caller,
	}
	return keeper
}

// Codespace returns the codespace
func (k Keeper) Codespace() sdk.CodespaceType {
	return k.codespace
}

// Logger returns a module-specific logger
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", types.ModuleName)
}

// GetSpanKey appends prefix to start block
func GetSpanKey(id uint64) []byte {
	return append(SpanPrefixKey, []byte(strconv.FormatUint(id, 10))...)
}

// AddNewSpan adds new span for bor to store
func (k *Keeper) AddNewSpan(ctx sdk.Context, span hmTypes.Span) error {
	store := ctx.KVStore(k.storeKey)
	out, err := k.cdc.MarshalBinaryBare(span)
	if err != nil {
		k.Logger(ctx).Error("Error marshalling span", "error", err)
		return err
	}

	// store set span id
	store.Set(GetSpanKey(span.ID), out)

	// update last span
	k.UpdateLastSpan(ctx, span.ID)
	return nil
}

// AddNewRawSpan adds new span for bor to store
func (k *Keeper) AddNewRawSpan(ctx sdk.Context, span hmTypes.Span) error {
	store := ctx.KVStore(k.storeKey)
	out, err := k.cdc.MarshalBinaryBare(span)
	if err != nil {
		k.Logger(ctx).Error("Error marshalling span", "error", err)
		return err
	}
	store.Set(GetSpanKey(span.ID), out)
	return nil
}

// GetSpan fetches span indexed by id from store
func (k *Keeper) GetSpan(ctx sdk.Context, id uint64) (*hmTypes.Span, error) {
	store := ctx.KVStore(k.storeKey)
	spanKey := GetSpanKey(id)

	// If we are starting from 0 there will be no spanKey present
	if !store.Has(spanKey) {
		return nil, errors.New("span not found for id")
	}

	var span hmTypes.Span
	if err := k.cdc.UnmarshalBinaryBare(store.Get(spanKey), &span); err != nil {
		return nil, err
	}

	return &span, nil
}

// GetAllSpans fetches all indexed by id from store
func (k *Keeper) GetAllSpans(ctx sdk.Context) (spans []*hmTypes.Span) {
	// iterate through spans and create span update array
	k.IterateSpansAndApplyFn(ctx, func(span hmTypes.Span) error {
		// append to list of validatorUpdates
		spans = append(spans, &span)
		return nil
	})

	return
}

// GetSpanList returns all spans with params like page and limit
func (k *Keeper) GetSpanList(ctx sdk.Context, page uint64, limit uint64) ([]hmTypes.Span, error) {
	store := ctx.KVStore(k.storeKey)

	// create spans
	var spans []hmTypes.Span

	// have max limit
	if limit > 20 {
		limit = 20
	}

	// get paginated iterator
	iterator := hmTypes.KVStorePrefixIteratorPaginated(store, SpanPrefixKey, uint(page), uint(limit))

	// loop through validators to get valid validators
	for ; iterator.Valid(); iterator.Next() {
		var span hmTypes.Span
		if err := k.cdc.UnmarshalBinaryBare(iterator.Value(), &span); err == nil {
			spans = append(spans, span)
		}
	}

	return spans, nil
}

// GetLastSpan fetches last span using lastStartBlock
func (k *Keeper) GetLastSpan(ctx sdk.Context) (*hmTypes.Span, error) {
	store := ctx.KVStore(k.storeKey)

	var lastSpanID uint64
	if store.Has(LastSpanIDKey) {
		// get last span id
		var err error
		lastSpanID, err = strconv.ParseUint(string(store.Get(LastSpanIDKey)), 10, 64)
		if err != nil {
			return nil, err
		}
	}

	return k.GetSpan(ctx, lastSpanID)
}

// FreezeSet freezes validator set for next span
func (k *Keeper) FreezeSet(ctx sdk.Context, id uint64, startBlock uint64, borChainID string) error {
	duration := k.GetSpanDuration(ctx)
	endBlock := startBlock
	if duration > 0 {
		endBlock = endBlock + duration - 1
	}

	// select next producers
	newProducers, err := k.SelectNextProducers(ctx)
	if err != nil {
		return err
	}

	// increment last eth block
	k.IncrementLastEthBlock(ctx)

	// generate new span
	newSpan := hmTypes.NewSpan(
		id,
		startBlock,
		endBlock,
		k.sk.GetValidatorSet(ctx),
		newProducers,
		borChainID,
	)

	return k.AddNewSpan(ctx, newSpan)
}

// SelectNextProducers selects producers for next span
func (k *Keeper) SelectNextProducers(ctx sdk.Context) (vals []hmTypes.Validator, err error) {
	// fetch last block used for seed
	lastEthBlock := k.GetLastEthBlock(ctx)

	// spanEligibleVals are current validators who are not getting deactivated in between next span
	spanEligibleVals := k.sk.GetSpanEligibleValidators(ctx)
	producerCount, err := k.GetProducerCount(ctx)
	if err != nil {
		return vals, err
	}

	// if producers to be selected is more than current validators no need to select/shuffle
	if len(spanEligibleVals) <= int(producerCount) {
		return spanEligibleVals, nil
	}

	// increment last processed header block number
	newEthBlock := lastEthBlock.Add(lastEthBlock, big.NewInt(1))

	// fetch block header from mainchain
	blockHeader, err := k.contractCaller.GetMainChainBlock(newEthBlock)
	if err != nil {
		return vals, err
	}

	// select next producers using seed as blockheader hash
	newProducersIds, err := SelectNextProducers(blockHeader.Hash(), spanEligibleVals, producerCount)
	if err != nil {
		return vals, err
	}

	IDToPower := make(map[uint64]uint64)
	for _, ID := range newProducersIds {
		IDToPower[ID] = IDToPower[ID] + 1
	}

	for key, value := range IDToPower {
		if val, ok := k.sk.GetValidatorFromValID(ctx, hmTypes.NewValidatorID(key)); ok {
			val.VotingPower = int64(value)
			vals = append(vals, val)
		}
	} // sort by address
	vals = hmTypes.SortValidatorByAddress(vals)

	return vals, nil
}

// UpdateLastSpan updates the last span start block
func (k *Keeper) UpdateLastSpan(ctx sdk.Context, id uint64) {
	store := ctx.KVStore(k.storeKey)
	store.Set(LastSpanIDKey, []byte(strconv.FormatUint(id, 10)))
}

// IncrementLastEthBlock increment last eth block
func (k *Keeper) IncrementLastEthBlock(ctx sdk.Context) {
	store := ctx.KVStore(k.storeKey)
	lastEthBlock := big.NewInt(0)
	if store.Has(LastProcessedEthBlock) {
		lastEthBlock = lastEthBlock.SetBytes(store.Get(LastProcessedEthBlock))
	}
	store.Set(LastProcessedEthBlock, lastEthBlock.Add(lastEthBlock, big.NewInt(1)).Bytes())
}

// SetLastEthBlock sets last eth block number
func (k *Keeper) SetLastEthBlock(ctx sdk.Context, blockNumber *big.Int) {
	store := ctx.KVStore(k.storeKey)
	store.Set(LastProcessedEthBlock, blockNumber.Bytes())
}

// GetLastEthBlock get last processed Eth block for seed
func (k *Keeper) GetLastEthBlock(ctx sdk.Context) *big.Int {
	store := ctx.KVStore(k.storeKey)
	lastEthBlock := big.NewInt(0)
	if store.Has(LastProcessedEthBlock) {
		lastEthBlock = lastEthBlock.SetBytes(store.Get(LastProcessedEthBlock))
	}
	return lastEthBlock
}

//
//  Params
//

// GetSpanDuration returns the span duration
func (k *Keeper) GetSpanDuration(ctx sdk.Context) uint64 {
	var duration uint64
	k.paramSpace.Get(ctx, types.ParamStoreKeySpanDuration, &duration)
	return duration
}

// SetSpanDuration sets the span duration
func (k *Keeper) SetSpanDuration(ctx sdk.Context, duration uint64) {
	k.paramSpace.Set(ctx, types.ParamStoreKeySpanDuration, duration)
}

// GetSprintDuration returns the span duration
func (k *Keeper) GetSprintDuration(ctx sdk.Context) uint64 {
	var duration uint64
	k.paramSpace.Get(ctx, types.ParamStoreKeySprintDuration, &duration)
	return duration
}

// SetSprintDuration sets the sprint duration
func (k *Keeper) SetSprintDuration(ctx sdk.Context, duration uint64) {
	k.paramSpace.Set(ctx, types.ParamStoreKeySprintDuration, duration)
}

// GetProducerCount returns the numeber of producers per span
func (k *Keeper) GetProducerCount(ctx sdk.Context) (uint64, error) {
	var count uint64
	if k.paramSpace.Has(ctx, types.ParamStoreKeyNumOfProducers) {
		k.paramSpace.Get(ctx, types.ParamStoreKeyNumOfProducers, &count)
	} else {
		return count, errors.New("producer count store key not found")
	}
	return count, nil
}

// SetProducerCount sets the number of producers selected per span
func (k *Keeper) SetProducerCount(ctx sdk.Context, count uint64) {
	k.paramSpace.Set(ctx, types.ParamStoreKeyNumOfProducers, count)
}

//
// Utils
//

// IterateSpansAndApplyFn interate spans and apply the given function.
func (k *Keeper) IterateSpansAndApplyFn(ctx sdk.Context, f func(span hmTypes.Span) error) {
	store := ctx.KVStore(k.storeKey)

	// get span iterator
	iterator := sdk.KVStorePrefixIterator(store, SpanPrefixKey)
	defer iterator.Close()

	// loop through spans to get valid spans
	for ; iterator.Valid(); iterator.Next() {
		// unmarshall span
		var result hmTypes.Span
		k.cdc.UnmarshalBinaryBare(iterator.Value(), &result)
		// call function and return if required
		if err := f(result); err != nil {
			return
		}
	}
}
