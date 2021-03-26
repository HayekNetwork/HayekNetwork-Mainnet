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

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/hayekchain/go-hayekchain/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("hyk/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("hyk/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("hyk/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("hyk/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("hyk/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("hyk/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("hyk/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("hyk/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("hyk/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("hyk/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("hyk/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("hyk/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("hyk/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("hyk/downloader/states/drop", nil)

	throttleCounter = metrics.NewRegisteredCounter("hyk/downloader/throttle", nil)
)
