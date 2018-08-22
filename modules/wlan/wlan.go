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

// Package wlan provides an i3bar module for wireless information.
// NOTE: This module REQUIRES the external command "iwgetid",
// because getting the SSID is a privileged operation.
package wlan

import (
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/soumya92/barista/bar"
	"github.com/soumya92/barista/base"
	"github.com/soumya92/barista/base/watchers/netlink"
	l "github.com/soumya92/barista/logging"
)

// Info represents the wireless card status.
type Info struct {
	Name           string
	State          netlink.OperState
	IPs            []net.IP
	SSID           string
	AccessPointMAC string
	Channel        int
	Frequency      float64
}

// Connecting returns true if a connection is in progress.
func (i Info) Connecting() bool {
	return i.State == netlink.Dormant
}

// Connected returns true if connected to a wireless network.
func (i Info) Connected() bool {
	return i.State == netlink.Up
}

// Enabled returns true if the wireless card is enabled.
func (i Info) Enabled() bool {
	return i.State > netlink.NotPresent
}

// Module represents a wlan bar module.
type Module struct {
	intf       string
	outputFunc base.Value // of func(Info) bar.Output
}

// Named constructs an instance of the wlan module for the specified interface.
func Named(iface string) *Module {
	m := &Module{intf: iface}
	l.Label(m, iface)
	l.Register(m, "outputFunc")
	// Default output template is just the SSID when connected.
	m.Template("{{if .Connected}}{{.SSID}}{{end}}")
	return m
}

// Any constructs an instance of the wlan module that uses any available
// wireless interface, choosing the 'best' state from all available.
func Any() *Module {
	return Named("")
}

// Output configures a module to display the output of a user-defined function.
func (m *Module) Output(outputFunc func(Info) bar.Output) *Module {
	m.outputFunc.Set(outputFunc)
	return m
}

// Template configures a module to display the output of a template.
func (m *Module) Template(template string) *Module {
	base.Template(template, m.Output)
	return m
}

// Stream starts the module.
func (m *Module) Stream(s bar.Sink) {
	info := Info{}
	outputFunc := m.outputFunc.Get().(func(Info) bar.Output)
	var updateChan netlink.Subscription
	if m.intf == "" {
		updateChan = netlink.WithPrefix("wl")
	} else {
		updateChan = netlink.ByName(m.intf)
	}
	defer updateChan.Unsubscribe()
	for {
		select {
		case update := <-updateChan:
			info = Info{
				Name:  update.Name,
				State: update.State,
				IPs:   update.IPs,
			}
			fillWifiInfo(&info)
		case <-m.outputFunc.Update():
			outputFunc = m.outputFunc.Get().(func(Info) bar.Output)
		}
		s.Output(outputFunc(info))
	}
}

func fillWifiInfo(info *Info) {
	ssid, err := iwgetid(info.Name, "-r")
	if err != nil {
		return
	}
	info.SSID = ssid
	info.AccessPointMAC, _ = iwgetid(info.Name, "-a")
	ch, _ := iwgetid(info.Name, "-c")
	info.Channel, _ = strconv.Atoi(ch)
	freq, _ := iwgetid(info.Name, "-f")
	info.Frequency, _ = strconv.ParseFloat(freq, 64)
}

var iwgetid = func(intf, flag string) (string, error) {
	out, err := exec.Command("/sbin/iwgetid", intf, "-r", flag).Output()
	return strings.TrimSpace(string(out)), err
}
