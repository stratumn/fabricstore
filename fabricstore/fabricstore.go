// Copyright 2017 Stratumn SAS. All rights reserved.
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

// Package fabricstore implements a store that saves all the segments in a
// Hyperledger Fabric distributed ledger
package fabricstore

import (
	"encoding/json"

	fab "github.com/hyperledger/fabric-sdk-go/api/apifabclient"
	"github.com/hyperledger/fabric-sdk-go/api/apitxn"
	"github.com/hyperledger/fabric-sdk-go/def/fabapi"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/events"
	common "github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"

	"github.com/stratumn/sdk/cs"
	"github.com/stratumn/sdk/store"
	"github.com/stratumn/sdk/types"

	log "github.com/sirupsen/logrus"
)

const (
	// Name is the name set in the store's information.
	Name = "fabric"

	// Description is the description set in the store's information.
	Description = "Stratumn Fabric Store"
)

// Config contains configuration options for the store
type Config struct {
	// ChannelID used to send transactions
	ChannelID string

	// ChaincodeID used for transactions
	ChaincodeID string

	// ConfigFile path to network configuration file (yaml)
	ConfigFile string

	// A version string that will be set in the store's information.
	Version string

	// A git commit hash that will be set in the store's information.
	Commit string
}

// FabricStore is the type that implements github.com/stratumn/sdk/store.Adapter.
type FabricStore struct {
	session      *fabapi.Session
	fabricClient apitxn.ChannelClient

	// channelClient is used to send transaction proposals.
	channelClient apitxn.ChannelClient

	// peerClient is the client connection to the peer.
	peerClient fab.FabricClient

	// eventHub is used to listen for new block events
	eventHub fab.EventHub

	didSaveChans    []chan *cs.Segment
	fabricEventChan chan *apitxn.CCEvent
	config          *Config
}

// Info is the info returned by GetInfo.
type Info struct {
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	FabricAppInfo interface{} `json:"fabricAppDescription"`
	Version       string      `json:"version"`
	Commit        string      `json:"commit"`
}

// New creates a new instance of FabricStore
func New(config *Config) (*FabricStore, error) {
	sdkOptions := fabapi.Options{
		ConfigFile: config.ConfigFile,
	}

	sdk, err := fabapi.NewSDK(sdkOptions)
	if err != nil {
		return nil, err
	}

	chClient, err := sdk.NewChannelClient(config.ChannelID, "Admin")
	if err != nil {
		return nil, err
	}

	client, err := sdk.ConfigProvider().Client()
	if err != nil {
		return nil, err
	}

	session, err := sdk.NewPreEnrolledUserSession(client.Organization, "Admin")
	if err != nil {
		return nil, err
	}

	peerClient, err := sdk.NewSystemClient(session)
	if err != nil {
		return nil, err
	}

	eventHub, err := getEventHub(peerClient)
	if err != nil {
		return nil, err
	}

	adapter := &FabricStore{
		fabricClient:    chClient,
		channelClient:   chClient,
		session:         session,
		peerClient:      peerClient,
		eventHub:        eventHub,
		config:          config,
		fabricEventChan: make(chan *apitxn.CCEvent, 256),
	}

	// Listen to block events
	if err := adapter.listenToBlockEvents(); err != nil {
		return nil, err
	}

	// Register to saveSegment events
	chClient.RegisterChaincodeEvent(adapter.fabricEventChan, config.ChaincodeID, "saveSegment")

	// Start litening to events
	go adapter.Listen()

	return adapter, nil
}

// Listen starts listening to fabric saveSegment events
func (f *FabricStore) Listen() {
	for {
		select {
		case evt := <-f.fabricEventChan:
			segment := &cs.Segment{}
			json.Unmarshal(evt.Payload, segment)
			for _, c := range f.didSaveChans {
				c <- segment
			}
		}
	}
}

// AddDidSaveChannel implements
// github.com/stratumn/sdk/fossilizer.Store.AddDidSaveChannel.
func (f *FabricStore) AddDidSaveChannel(saveChan chan *cs.Segment) {
	f.didSaveChans = append(f.didSaveChans, saveChan)
}

// GetInfo implements github.com/stratumn/sdk/store.Adapter.GetInfo.
func (f *FabricStore) GetInfo() (interface{}, error) {
	return &Info{
		Name:          Name,
		Description:   Description,
		FabricAppInfo: nil,
		Version:       f.config.Version,
		Commit:        f.config.Commit,
	}, nil
}

// SaveSegment implements github.com/stratumn/sdk/store.Adapter.SaveSegment.
func (f *FabricStore) SaveSegment(segment *cs.Segment) error {
	segmentBytes, _ := json.Marshal(segment)

	_, err := f.fabricClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "SaveSegment",
		Args:        [][]byte{segmentBytes},
	})

	return err
}

// GetSegment implements github.com/stratumn/sdk/store.Adapter.GetSegment.
func (f *FabricStore) GetSegment(linkHash *types.Bytes32) (segment *cs.Segment, err error) {
	response, err := f.fabricClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "GetSegment",
		Args:        [][]byte{[]byte(linkHash.String())},
	})
	if err != nil {
		return
	}
	if response == nil {
		return
	}

	segment = &cs.Segment{}
	err = json.Unmarshal(response, segment)
	if err != nil {
		return
	}

	if segment.IsEmpty() {
		segment = nil
	}
	return
}

// DeleteSegment implements github.com/stratumn/sdk/store.Adapter.DeleteSegment.
func (f *FabricStore) DeleteSegment(linkHash *types.Bytes32) (segment *cs.Segment, err error) {
	segment, err = f.GetSegment(linkHash)
	if err != nil {
		return
	}

	_, err = f.fabricClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "DeleteSegment",
		Args:        [][]byte{[]byte(linkHash.String())},
	})
	if err != nil {
		return
	}

	return
}

// FindSegments implements github.com/stratumn/sdk/store.Adapter.FindSegments.
func (f *FabricStore) FindSegments(filter *store.SegmentFilter) (segmentSlice cs.SegmentSlice, err error) {
	filterBytes, _ := json.Marshal(filter)

	response, err := f.fabricClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "FindSegments",
		Args:        [][]byte{filterBytes},
	})
	if err != nil {
		return
	}

	err = json.Unmarshal(response, &segmentSlice)
	if err != nil {
		return
	}

	// This should be removed once limit and skip are implemented in fabric/couchDB
	segmentSlice = filter.Pagination.PaginateSegments(segmentSlice)

	return
}

// GetMapIDs implements github.com/stratumn/sdk/store.Adapter.GetMapIDs.
func (f *FabricStore) GetMapIDs(filter *store.MapFilter) (ids []string, err error) {
	filterBytes, _ := json.Marshal(filter)

	response, err := f.fabricClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "GetMapIDs",
		Args:        [][]byte{filterBytes},
	})
	if err != nil {
		return
	}

	err = json.Unmarshal(response, &ids)
	if err != nil {
		return
	}

	// This should be removed once limit and skip are implemented in fabric/couchDB
	ids = filter.Pagination.PaginateStrings(ids)

	return
}

// NewBatch implements github.com/stratumn/sdk/store.Adapter.NewBatch.
func (f *FabricStore) NewBatch() (store.Batch, error) {
	return NewBatch(f), nil
}

// SaveValue implements github.com/stratumn/sdk/store.Adapter.SaveValue.
func (f *FabricStore) SaveValue(key, value []byte) error {
	_, err := f.fabricClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "SaveValue",
		Args:        [][]byte{key, value},
	})

	return err
}

// GetValue implements github.com/stratumn/sdk/store.Adapter.GetValue.
func (f *FabricStore) GetValue(key []byte) (value []byte, err error) {
	response, err := f.fabricClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "GetValue",
		Args:        [][]byte{key},
	})
	if err != nil {
		return nil, err
	}

	return response, nil
}

// DeleteValue implements github.com/stratumn/sdk/store.Adapter.DeleteValue.
func (f *FabricStore) DeleteValue(key []byte) (value []byte, err error) {
	value, err = f.GetValue(key)
	if err != nil {
		return nil, err
	}

	_, err = f.fabricClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "DeleteValue",
		Args:        [][]byte{key},
	})

	return
}

func (f *FabricStore) listenToBlockEvents() error {
	if err := f.eventHub.Connect(); err != nil {
		return err
	}

	f.eventHub.RegisterBlockEvent(f.onBlock)
	return nil
}

// onBlock is the callback function called on block events.
func (f *FabricStore) onBlock(block *common.Block) {
	log.Infof("Received block")
	// if err := decodeBlock(block); err != nil {
	// 	t.Logf(err.Error())
	// 	t.FailNow()
	// }
}

func getEventHub(client fab.FabricClient) (fab.EventHub, error) {
	eventHub, err := events.NewEventHub(client)
	if err != nil {
		return nil, err
	}
	foundEventHub := false
	peerConfig, err := client.Config().PeersConfig("Org1")
	if err != nil {
		return nil, err
	}
	for _, p := range peerConfig {
		if p.URL != "" {
			log.Infof("EventHub connect to peer (%s)", p.URL)
			serverHostOverride := ""
			if str, ok := p.GRPCOptions["ssl-target-name-override"].(string); ok {
				serverHostOverride = str
			}
			eventHub.SetPeerAddr(p.EventURL, p.TLSCACerts.Path, serverHostOverride)
			foundEventHub = true
			break
		}
	}

	if !foundEventHub {
		return nil, err
	}

	return eventHub, nil
}
