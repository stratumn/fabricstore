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
	"sort"

	fab "github.com/hyperledger/fabric-sdk-go/api/apifabclient"
	"github.com/hyperledger/fabric-sdk-go/api/apitxn"
	"github.com/hyperledger/fabric-sdk-go/def/fabapi"
	common "github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"

	pc "github.com/stratumn/fabricstore/chaincode/pop/popconfig"

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
	config        *Config
	evidenceStore store.EvidenceStore
	didSaveChans  []chan *cs.Segment
	eventChans    []chan *store.Event

	// client is the client connection to the organization.
	client fab.FabricClient

	// channelClient is used to send transaction proposals.
	channelClient apitxn.ChannelClient

	// channel is used to query blocks.
	channel fab.Channel

	// eventHub is used to listen for new block events
	eventHub fab.EventHub
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
func New(e store.EvidenceStore, config *Config) (*FabricStore, error) {
	sdkOptions := fabapi.Options{
		ConfigFile: config.ConfigFile,
	}

	sdk, err := fabapi.NewSDK(sdkOptions)
	if err != nil {
		return nil, err
	}

	clientConfig, err := sdk.ConfigProvider().Client()
	if err != nil {
		return nil, err
	}

	session, err := sdk.NewPreEnrolledUserSession(clientConfig.Organization, "Admin")
	if err != nil {
		return nil, err
	}

	client, err := sdk.NewSystemClient(session)
	if err != nil {
		return nil, err
	}

	channelClient, err := sdk.NewChannelClient(config.ChannelID, "Admin")
	if err != nil {
		return nil, err
	}

	channel, err := getChannel(client, config.ChannelID, clientConfig.Organization)
	if err != nil {
		return nil, err
	}

	eventHub, err := getEventHub(client, clientConfig.Organization)
	if err != nil {
		return nil, err
	}

	adapter := &FabricStore{
		config:        config,
		evidenceStore: e,
		client:        client,
		channelClient: channelClient,
		channel:       channel,
		eventHub:      eventHub,
	}

	// Listen to block events
	if err := adapter.listenToBlockEvents(); err != nil {
		return nil, err
	}

	return adapter, nil
}

// AddDidSaveChannel implements
// github.com/stratumn/sdk/fossilizer.Store.AddDidSaveChannel.
func (f *FabricStore) AddDidSaveChannel(saveChan chan *cs.Segment) {
	f.didSaveChans = append(f.didSaveChans, saveChan)
}

// AddStoreEventChannel implements github.com/stratumn/sdk/store.AdapterV2.AddStoreEventChannel
func (f *FabricStore) AddStoreEventChannel(eventChan chan *store.Event) {
	f.eventChans = append(f.eventChans, eventChan)
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
	linkHash, err := f.CreateLink(&segment.Link)
	if err != nil {
		return err
	}

	for _, evidence := range segment.Meta.Evidences {
		if err := f.AddEvidence(linkHash, evidence); err != nil {
			return err
		}
	}

	return nil
}

// CreateLink implements github.com/stratumn/sdk/store.LinkWriter.CreateLink.
func (f *FabricStore) CreateLink(link *cs.Link) (*types.Bytes32, error) {
	linkHash, err := link.Hash()
	if err != nil {
		return nil, err
	}

	linkBytes, _ := json.Marshal(link)

	_, err = f.channelClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         "CreateLink",
		Args:        [][]byte{linkBytes},
	})
	return linkHash, nil
}

// GetSegment implements github.com/stratumn/sdk/store.Adapter.GetSegment.
func (f *FabricStore) GetSegment(linkHash *types.Bytes32) (*cs.Segment, error) {
	response, err := f.channelClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.GetLink,
		Args:        [][]byte{[]byte(linkHash.String())},
	})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}

	link := cs.Link{}
	err = json.Unmarshal(response, &link)
	if err != nil {
		return nil, err
	}

	return f.buildSegment(link)
}

// DeleteSegment implements github.com/stratumn/sdk/store.Adapter.DeleteSegment.
func (f *FabricStore) DeleteSegment(linkHash *types.Bytes32) (segment *cs.Segment, err error) {
	segment, err = f.GetSegment(linkHash)
	if err != nil {
		return
	}

	_, err = f.channelClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.DeleteLink,
		Args:        [][]byte{[]byte(linkHash.String())},
	})
	if err != nil {
		return
	}

	return
}

// AddEvidence implements github.com/stratumn/sdk/store.EvidenceWriter.AddEvidence.
func (f *FabricStore) AddEvidence(linkHash *types.Bytes32, evidence *cs.Evidence) error {
	return f.evidenceStore.AddEvidence(linkHash, evidence)
}

// GetEvidences implements github.com/stratumn/sdk/store.EvidenceReader.GetEvidences.
func (f *FabricStore) GetEvidences(linkHash *types.Bytes32) (*cs.Evidences, error) {
	return f.evidenceStore.GetEvidences(linkHash)
}

// FindSegments implements github.com/stratumn/sdk/store.Adapter.FindSegments.
func (f *FabricStore) FindSegments(filter *store.SegmentFilter) (segmentSlice cs.SegmentSlice, err error) {
	filterBytes, _ := json.Marshal(filter)

	response, err := f.channelClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.FindLinks,
		Args:        [][]byte{filterBytes},
	})
	if err != nil {
		return
	}
	links := []cs.Link{}
	err = json.Unmarshal(response, &links)
	if err != nil {
		return
	}

	for _, link := range links {
		segment, _ := f.buildSegment(link)
		segmentSlice = append(segmentSlice, segment)
	}

	sort.Sort(segmentSlice)

	// This should be removed once limit and skip are implemented in fabric/couchDB
	segmentSlice = filter.Pagination.PaginateSegments(segmentSlice)

	return
}

// GetMapIDs implements github.com/stratumn/sdk/store.Adapter.GetMapIDs.
func (f *FabricStore) GetMapIDs(filter *store.MapFilter) (ids []string, err error) {
	filterBytes, _ := json.Marshal(filter)

	response, err := f.channelClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.GetMapIDs,
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

// NewBatchV2 implements github.com/stratumn/sdk/store.AdapterV2.NewBatchV2.
func (f *FabricStore) NewBatchV2() (store.BatchV2, error) {
	return nil, nil
}

// SaveValue implements github.com/stratumn/sdk/store.Adapter.SaveValue.
func (f *FabricStore) SaveValue(key, value []byte) error {
	_, err := f.channelClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.SaveValue,
		Args:        [][]byte{key, value},
	})

	return err
}

// GetValue implements github.com/stratumn/sdk/store.Adapter.GetValue.
func (f *FabricStore) GetValue(key []byte) (value []byte, err error) {
	response, err := f.channelClient.Query(apitxn.QueryRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.GetValue,
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

	_, err = f.channelClient.ExecuteTx(apitxn.ExecuteTxRequest{
		ChaincodeID: f.config.ChaincodeID,
		Fcn:         pc.DeleteValue,
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
	log.Infof("Received block %v", block.Header.Number)
	transactions, err := readBlock(block)
	if err != nil {
		panic(err)
	}
	for _, tx := range transactions {
		if tx.Action == "CreateLink" {
			link := cs.Link{}
			if err := json.Unmarshal(tx.Args[0], &link); err != nil {
				panic(err)
			}

			// TODO generate new fabricstore evidence

			segment, _ := f.buildSegment(link)

			for _, c := range f.didSaveChans {
				c <- segment
			}

			for _, c := range f.eventChans {
				c <- &store.Event{
					EventType: store.SavedLink,
					Details:   link,
				}
			}
		}
	}
}

func (f *FabricStore) buildSegment(link cs.Link) (*cs.Segment, error) {
	linkHash, err := link.Hash()
	if err != nil {
		return nil, err
	}

	evidences, err := f.GetEvidences(linkHash)
	if err != nil {
		return nil, err
	}

	return &cs.Segment{
		Link: link,
		Meta: cs.SegmentMeta{
			Evidences: *evidences,
			LinkHash:  linkHash.String(),
		},
	}, nil
}
