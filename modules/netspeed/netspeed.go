// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package netspeed provides an i3bar module to display network utilisation.
package netspeed

import (
	"time"

	"github.com/martinlindhe/unit"
	"github.com/vishvananda/netlink"

	"github.com/soumya92/barista"
	"github.com/soumya92/barista/bar"
	"github.com/soumya92/barista/base"
	"github.com/soumya92/barista/outputs"
	"github.com/soumya92/barista/scheduler"
)

// Speeds represents bidirectional network traffic.
type Speeds struct {
	Rx, Tx unit.Datarate
	// Keep track of whether these speeds are actually 0
	// or uninitialised.
	available bool
}

// Total gets the total speed (both up and down).
func (s Speeds) Total() unit.Datarate {
	return s.Rx + s.Tx
}

// Module represents a netspeed bar module. It supports setting the output
// format, click handler, and update frequency.
type Module interface {
	base.SimpleClickHandlerModule

	// RefreshInterval configures the polling frequency for network speed.
	// Since there is no concept of an instantaneous network speed, the speeds will
	// be averaged over this interval before being displayed.
	RefreshInterval(time.Duration) Module

	// OutputFunc configures a module to display the output of a user-defined function.
	OutputFunc(func(Speeds) bar.Output) Module

	// OutputTemplate configures a module to display the output of a template.
	OutputTemplate(func(interface{}) bar.Output) Module
}

type module struct {
	base.SimpleClickHandler
	iface      string
	scheduler  bar.Scheduler
	outputFunc base.Value // of func(Speeds) bar.Output
}

// New constructs an instance of the netspeed module for the given interface.
func New(iface string) Module {
	m := &module{
		iface:     iface,
		scheduler: barista.Schedule(),
	}
	// Default is to refresh every 3s, similar to top.
	m.RefreshInterval(3 * time.Second)
	// Default output template that's just the up and down speeds in SI.
	m.OutputTemplate(outputs.TextTemplate("{{.Tx | ibyterate}} up | {{.Rx | ibyterate}} down"))
	return m
}

func (m *module) OutputFunc(outputFunc func(Speeds) bar.Output) Module {
	m.outputFunc.Set(outputFunc)
	return m
}

func (m *module) OutputTemplate(template func(interface{}) bar.Output) Module {
	return m.OutputFunc(func(s Speeds) bar.Output {
		return template(s)
	})
}

func (m *module) RefreshInterval(interval time.Duration) Module {
	m.scheduler.Every(interval)
	return m
}

func (m *module) Stream() <-chan bar.Output {
	ch := base.NewChannel()
	go m.worker(ch)
	return ch
}

// For tests.
var linkByName = netlink.LinkByName

func (m *module) worker(ch base.Channel) {
	lastRx, lastTx, err := linkRxTx(m.iface)
	if ch.Error(err) {
		return
	}
	lastRead := scheduler.Now()

	var speeds Speeds
	outputFunc := m.outputFunc.Get().(func(Speeds) bar.Output)
	sOutputFunc := m.outputFunc.Subscribe()

	for {
		if speeds.available {
			ch.Output(outputFunc(speeds))
		}
		select {
		case <-sOutputFunc.Tick():
			outputFunc = m.outputFunc.Get().(func(Speeds) bar.Output)
		case <-m.scheduler.Tick():
			rx, tx, err := linkRxTx(m.iface)
			if ch.Error(err) {
				return
			}
			now := scheduler.Now()
			duration := now.Sub(lastRead).Seconds()

			speeds.available = true
			speeds.Rx = unit.Datarate(float64(rx-lastRx)/duration) * unit.BytePerSecond
			speeds.Tx = unit.Datarate(float64(tx-lastTx)/duration) * unit.BytePerSecond

			lastRead = now
			lastRx = rx
			lastTx = tx
		}
	}
}

func linkRxTx(iface string) (rx, tx uint64, err error) {
	var link netlink.Link
	link, err = linkByName(iface)
	if err != nil {
		return
	}
	linkStats := link.Attrs().Statistics
	rx = linkStats.RxBytes
	tx = linkStats.TxBytes
	return
}
