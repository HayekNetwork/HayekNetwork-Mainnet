// Copyright 2019 The go-hayekchain Authors
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
	"github.com/hayekchain/go-hayekchain/core"
	"github.com/hayekchain/go-hayekchain/core/forkid"
	"github.com/hayekchain/go-hayekchain/p2p/dnsdisc"
	"github.com/hayekchain/go-hayekchain/p2p/enode"
	"github.com/hayekchain/go-hayekchain/rlp"
)

// ethEntry is the "hyk" ENR entry which advertises hyk protocol
// on the discovery network.
type ethEntry struct {
	ForkID forkid.ID // Fork identifier per EIP-2124

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

// ENRKey implements enr.Entry.
func (e ethEntry) ENRKey() string {
	return "hyk"
}

// startHykEntryUpdate starts the ENR updater loop.
func (hyk *HayekChain) startHykEntryUpdate(ln *enode.LocalNode) {
	var newHead = make(chan core.ChainHeadEvent, 10)
	sub := hyk.blockchain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-newHead:
				ln.Set(hyk.currentHykEntry())
			case <-sub.Err():
				// Would be nice to sync with hyk.Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

func (hyk *HayekChain) currentHykEntry() *ethEntry {
	return &ethEntry{ForkID: forkid.NewID(hyk.blockchain.Config(), hyk.blockchain.Genesis().Hash(),
		hyk.blockchain.CurrentHeader().Number.Uint64())}
}

// setupDiscovery creates the node discovery source for the hyk protocol.
func (hyk *HayekChain) setupDiscovery() (enode.Iterator, error) {
	if len(hyk.config.DiscoveryURLs) == 0 {
		return nil, nil
	}
	client := dnsdisc.NewClient(dnsdisc.Config{})
	return client.NewIterator(hyk.config.DiscoveryURLs...)
}
