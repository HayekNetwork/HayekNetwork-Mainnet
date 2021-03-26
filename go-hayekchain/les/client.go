// Copyright 2016 The go-hayekchain Authors
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

// Package les implements the Light HayekChain Subprotocol.
package les

import (
	"fmt"
	"time"

	"github.com/hayekchain/go-hayekchain/accounts"
	"github.com/hayekchain/go-hayekchain/common"
	"github.com/hayekchain/go-hayekchain/common/hexutil"
	"github.com/hayekchain/go-hayekchain/common/mclock"
	"github.com/hayekchain/go-hayekchain/consensus"
	"github.com/hayekchain/go-hayekchain/core"
	"github.com/hayekchain/go-hayekchain/core/bloombits"
	"github.com/hayekchain/go-hayekchain/core/rawdb"
	"github.com/hayekchain/go-hayekchain/core/types"
	"github.com/hayekchain/go-hayekchain/event"
	"github.com/hayekchain/go-hayekchain/hyk"
	"github.com/hayekchain/go-hayekchain/hyk/downloader"
	"github.com/hayekchain/go-hayekchain/hyk/filters"
	"github.com/hayekchain/go-hayekchain/hyk/gasprice"
	"github.com/hayekchain/go-hayekchain/internal/hykapi"
	lpc "github.com/hayekchain/go-hayekchain/les/lespay/client"
	"github.com/hayekchain/go-hayekchain/light"
	"github.com/hayekchain/go-hayekchain/log"
	"github.com/hayekchain/go-hayekchain/node"
	"github.com/hayekchain/go-hayekchain/p2p"
	"github.com/hayekchain/go-hayekchain/p2p/enode"
	"github.com/hayekchain/go-hayekchain/params"
	"github.com/hayekchain/go-hayekchain/rpc"
)

type LightHayekChain struct {
	lesCommons

	peers          *serverPeerSet
	reqDist        *requestDistributor
	retriever      *retrieveManager
	odr            *LesOdr
	relay          *lesTxRelay
	handler        *clientHandler
	txPool         *light.TxPool
	blockchain     *light.LightChain
	serverPool     *serverPool
	valueTracker   *lpc.ValueTracker
	dialCandidates enode.Iterator
	pruner         *pruner

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend     *LesApiBackend
	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager
	netRPCService  *hykapi.PublicNetAPI

	p2pServer *p2p.Server
}

// New creates an instance of the light client.
func New(stack *node.Node, config *hyk.Config) (*LightHayekChain, error) {
	chainDb, err := stack.OpenDatabase("lightchaindata", config.DatabaseCache, config.DatabaseHandles, "hyk/db/chaindata/")
	if err != nil {
		return nil, err
	}
	lespayDb, err := stack.OpenDatabase("lespay", 0, 0, "hyk/db/lespay")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	peers := newServerPeerSet()
	lhyk := &LightHayekChain{
		lesCommons: lesCommons{
			genesis:     genesisHash,
			config:      config,
			chainConfig: chainConfig,
			iConfig:     light.DefaultClientIndexerConfig,
			chainDb:     chainDb,
			closeCh:     make(chan struct{}),
		},
		peers:          peers,
		eventMux:       stack.EventMux(),
		reqDist:        newRequestDistributor(peers, &mclock.System{}),
		accountManager: stack.AccountManager(),
		engine:         hyk.CreateConsensusEngine(stack, chainConfig, &config.Hayekash, nil, false, chainDb),
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   hyk.NewBloomIndexer(chainDb, params.BloomBitsBlocksClient, params.HelperTrieConfirmations),
		valueTracker:   lpc.NewValueTracker(lespayDb, &mclock.System{}, requestList, time.Minute, 1/float64(time.Hour), 1/float64(time.Hour*100), 1/float64(time.Hour*1000)),
		p2pServer:      stack.Server(),
	}
	peers.subscribe((*vtSubscription)(lhyk.valueTracker))

	dnsdisc, err := lhyk.setupDiscovery()
	if err != nil {
		return nil, err
	}
	lhyk.serverPool = newServerPool(lespayDb, []byte("serverpool:"), lhyk.valueTracker, dnsdisc, time.Second, nil, &mclock.System{}, config.UltraLightServers)
	peers.subscribe(lhyk.serverPool)
	lhyk.dialCandidates = lhyk.serverPool.dialIterator

	lhyk.retriever = newRetrieveManager(peers, lhyk.reqDist, lhyk.serverPool.getTimeout)
	lhyk.relay = newLesTxRelay(peers, lhyk.retriever)

	lhyk.odr = NewLesOdr(chainDb, light.DefaultClientIndexerConfig, lhyk.retriever)
	lhyk.chtIndexer = light.NewChtIndexer(chainDb, lhyk.odr, params.CHTFrequency, params.HelperTrieConfirmations, config.LightNoPrune)
	lhyk.bloomTrieIndexer = light.NewBloomTrieIndexer(chainDb, lhyk.odr, params.BloomBitsBlocksClient, params.BloomTrieFrequency, config.LightNoPrune)
	lhyk.odr.SetIndexers(lhyk.chtIndexer, lhyk.bloomTrieIndexer, lhyk.bloomIndexer)

	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[genesisHash]
	}
	// Note: NewLightChain adds the trusted checkpoint so it needs an ODR with
	// indexers already set but not started yet
	if lhyk.blockchain, err = light.NewLightChain(lhyk.odr, lhyk.chainConfig, lhyk.engine, checkpoint); err != nil {
		return nil, err
	}
	lhyk.chainReader = lhyk.blockchain
	lhyk.txPool = light.NewTxPool(lhyk.chainConfig, lhyk.blockchain, lhyk.relay)

	// Set up checkpoint oracle.
	lhyk.oracle = lhyk.setupOracle(stack, genesisHash, config)

	// Note: AddChildIndexer starts the update process for the child
	lhyk.bloomIndexer.AddChildIndexer(lhyk.bloomTrieIndexer)
	lhyk.chtIndexer.Start(lhyk.blockchain)
	lhyk.bloomIndexer.Start(lhyk.blockchain)

	// Start a light chain pruner to delete useless historical data.
	lhyk.pruner = newPruner(chainDb, lhyk.chtIndexer, lhyk.bloomTrieIndexer)

	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		lhyk.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	lhyk.ApiBackend = &LesApiBackend{stack.Config().ExtRPCEnabled(), lhyk, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.Miner.GasPrice
	}
	lhyk.ApiBackend.gpo = gasprice.NewOracle(lhyk.ApiBackend, gpoParams)

	lhyk.handler = newClientHandler(config.UltraLightServers, config.UltraLightFraction, checkpoint, lhyk)
	if lhyk.handler.ulc != nil {
		log.Warn("Ultra light client is enabled", "trustedNodes", len(lhyk.handler.ulc.keys), "minTrustedFraction", lhyk.handler.ulc.fraction)
		lhyk.blockchain.DisableCheckFreq()
	}

	lhyk.netRPCService = hykapi.NewPublicNetAPI(lhyk.p2pServer, lhyk.config.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(lhyk.APIs())
	stack.RegisterProtocols(lhyk.Protocols())
	stack.RegisterLifecycle(lhyk)

	return lhyk, nil
}

// vtSubscription implements serverPeerSubscriber
type vtSubscription lpc.ValueTracker

// registerPeer implements serverPeerSubscriber
func (v *vtSubscription) registerPeer(p *serverPeer) {
	vt := (*lpc.ValueTracker)(v)
	p.setValueTracker(vt, vt.Register(p.ID()))
	p.updateVtParams()
}

// unregisterPeer implements serverPeerSubscriber
func (v *vtSubscription) unregisterPeer(p *serverPeer) {
	vt := (*lpc.ValueTracker)(v)
	vt.Unregister(p.ID())
	p.setValueTracker(nil, nil)
}

type LightDummyAPI struct{}

// Hayekerbase is the address that mining rewards will be send to
func (s *LightDummyAPI) Hayekerbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("mining is not supported in light mode")
}

// Coinbase is the address that mining rewards will be send to (alias for Hayekerbase)
func (s *LightDummyAPI) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("mining is not supported in light mode")
}

// Hashrate returns the POW hashrate
func (s *LightDummyAPI) Hashrate() hexutil.Uint {
	return 0
}

// Mining returns an indication if this node is currently mining.
func (s *LightDummyAPI) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the hayekchain package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *LightHayekChain) APIs() []rpc.API {
	apis := hykapi.GetAPIs(s.ApiBackend)
	apis = append(apis, s.engine.APIs(s.BlockChain().HeaderChain())...)
	return append(apis, []rpc.API{
		{
			Namespace: "hyk",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "hyk",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.handler.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "hyk",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, true),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		}, {
			Namespace: "les",
			Version:   "1.0",
			Service:   NewPrivateLightAPI(&s.lesCommons),
			Public:    false,
		}, {
			Namespace: "lespay",
			Version:   "1.0",
			Service:   lpc.NewPrivateClientAPI(s.valueTracker),
			Public:    false,
		},
	}...)
}

func (s *LightHayekChain) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *LightHayekChain) BlockChain() *light.LightChain      { return s.blockchain }
func (s *LightHayekChain) TxPool() *light.TxPool              { return s.txPool }
func (s *LightHayekChain) Engine() consensus.Engine           { return s.engine }
func (s *LightHayekChain) LesVersion() int                    { return int(ClientProtocolVersions[0]) }
func (s *LightHayekChain) Downloader() *downloader.Downloader { return s.handler.downloader }
func (s *LightHayekChain) EventMux() *event.TypeMux           { return s.eventMux }

// Protocols returns all the currently configured network protocols to start.
func (s *LightHayekChain) Protocols() []p2p.Protocol {
	return s.makeProtocols(ClientProtocolVersions, s.handler.runPeer, func(id enode.ID) interface{} {
		if p := s.peers.peer(id.String()); p != nil {
			return p.Info()
		}
		return nil
	}, s.dialCandidates)
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// light hayekchain protocol implementation.
func (s *LightHayekChain) Start() error {
	log.Warn("Light client mode is an experimental feature")

	s.serverPool.start()
	// Start bloom request workers.
	s.wg.Add(bloomServiceThreads)
	s.startBloomHandlers(params.BloomBitsBlocksClient)
	s.handler.start()

	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// HayekChain protocol.
func (s *LightHayekChain) Stop() error {
	close(s.closeCh)
	s.serverPool.stop()
	s.valueTracker.Stop()
	s.peers.close()
	s.reqDist.close()
	s.odr.Stop()
	s.relay.Stop()
	s.bloomIndexer.Close()
	s.chtIndexer.Close()
	s.blockchain.Stop()
	s.handler.stop()
	s.txPool.Stop()
	s.engine.Close()
	s.pruner.close()
	s.eventMux.Stop()
	s.chainDb.Close()
	s.wg.Wait()
	log.Info("Light hayekchain stopped")
	return nil
}
