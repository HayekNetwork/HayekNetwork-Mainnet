// Copyright 2015 The go-hayekchain Authors
// This file is part of the go-hayekchain library.
//
// The go-hayekchain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hayekchain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hayekchain library. If not, see <http://www.gnu.org/licenses/>.

package hyk

import (
	"context"
	"errors"
	"math/big"

	"github.com/hayekchain/go-hayekchain/accounts"
	"github.com/hayekchain/go-hayekchain/common"
	"github.com/hayekchain/go-hayekchain/consensus"
	"github.com/hayekchain/go-hayekchain/core"
	"github.com/hayekchain/go-hayekchain/core/bloombits"
	"github.com/hayekchain/go-hayekchain/core/rawdb"
	"github.com/hayekchain/go-hayekchain/core/state"
	"github.com/hayekchain/go-hayekchain/core/types"
	"github.com/hayekchain/go-hayekchain/core/vm"
	"github.com/hayekchain/go-hayekchain/event"
	"github.com/hayekchain/go-hayekchain/hyk/downloader"
	"github.com/hayekchain/go-hayekchain/hyk/gasprice"
	"github.com/hayekchain/go-hayekchain/hykdb"
	"github.com/hayekchain/go-hayekchain/miner"
	"github.com/hayekchain/go-hayekchain/params"
	"github.com/hayekchain/go-hayekchain/rpc"
)

// HykAPIBackend implements hykapi.Backend for full nodes
type HykAPIBackend struct {
	extRPCEnabled bool
	hyk           *HayekChain
	gpo           *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *HykAPIBackend) ChainConfig() *params.ChainConfig {
	return b.hyk.blockchain.Config()
}

func (b *HykAPIBackend) CurrentBlock() *types.Block {
	return b.hyk.blockchain.CurrentBlock()
}

func (b *HykAPIBackend) SetHead(number uint64) {
	b.hyk.protocolManager.downloader.Cancel()
	b.hyk.blockchain.SetHead(number)
}

func (b *HykAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.hyk.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.hyk.blockchain.CurrentBlock().Header(), nil
	}
	return b.hyk.blockchain.GetHeaderByNumber(uint64(number)), nil
}

func (b *HykAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.hyk.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.hyk.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *HykAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.hyk.blockchain.GetHeaderByHash(hash), nil
}

func (b *HykAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.hyk.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.hyk.blockchain.CurrentBlock(), nil
	}
	return b.hyk.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *HykAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.hyk.blockchain.GetBlockByHash(hash), nil
}

func (b *HykAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.hyk.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.hyk.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.hyk.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *HykAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, state := b.hyk.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.hyk.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *HykAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.hyk.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.hyk.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *HykAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.hyk.blockchain.GetReceiptsByHash(hash), nil
}

func (b *HykAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	receipts := b.hyk.blockchain.GetReceiptsByHash(hash)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *HykAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	return b.hyk.blockchain.GetTdByHash(hash)
}

func (b *HykAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header) (*vm.EVM, func() error, error) {
	vmError := func() error { return nil }

	txContext := core.NewEVMTxContext(msg)
	context := core.NewEVMBlockContext(header, b.hyk.BlockChain(), nil)
	return vm.NewEVM(context, txContext, state, b.hyk.blockchain.Config(), *b.hyk.blockchain.GetVMConfig()), vmError, nil
}

func (b *HykAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.hyk.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *HykAPIBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.hyk.miner.SubscribePendingLogs(ch)
}

func (b *HykAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.hyk.BlockChain().SubscribeChainEvent(ch)
}

func (b *HykAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.hyk.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *HykAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.hyk.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *HykAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.hyk.BlockChain().SubscribeLogsEvent(ch)
}

func (b *HykAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.hyk.txPool.AddLocal(signedTx)
}

func (b *HykAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.hyk.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *HykAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.hyk.txPool.Get(hash)
}

func (b *HykAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.hyk.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *HykAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.hyk.txPool.Nonce(addr), nil
}

func (b *HykAPIBackend) Stats() (pending int, queued int) {
	return b.hyk.txPool.Stats()
}

func (b *HykAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.hyk.TxPool().Content()
}

func (b *HykAPIBackend) TxPool() *core.TxPool {
	return b.hyk.TxPool()
}

func (b *HykAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.hyk.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *HykAPIBackend) Downloader() *downloader.Downloader {
	return b.hyk.Downloader()
}

func (b *HykAPIBackend) ProtocolVersion() int {
	return b.hyk.HykVersion()
}

func (b *HykAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *HykAPIBackend) ChainDb() hykdb.Database {
	return b.hyk.ChainDb()
}

func (b *HykAPIBackend) EventMux() *event.TypeMux {
	return b.hyk.EventMux()
}

func (b *HykAPIBackend) AccountManager() *accounts.Manager {
	return b.hyk.AccountManager()
}

func (b *HykAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *HykAPIBackend) RPCGasCap() uint64 {
	return b.hyk.config.RPCGasCap
}

func (b *HykAPIBackend) RPCTxFeeCap() float64 {
	return b.hyk.config.RPCTxFeeCap
}

func (b *HykAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.hyk.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *HykAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.hyk.bloomRequests)
	}
}

func (b *HykAPIBackend) Engine() consensus.Engine {
	return b.hyk.engine
}

func (b *HykAPIBackend) CurrentHeader() *types.Header {
	return b.hyk.blockchain.CurrentHeader()
}

func (b *HykAPIBackend) Miner() *miner.Miner {
	return b.hyk.Miner()
}

func (b *HykAPIBackend) StartMining(threads int) error {
	return b.hyk.StartMining(threads)
}
