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

package fabricstore

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stratumn/go-indigocore/cs"
	"github.com/stratumn/go-indigocore/cs/cstesting"
	"github.com/stratumn/go-indigocore/filestore"
	"github.com/stratumn/go-indigocore/store"

	_ "github.com/stratumn/fabricstore/evidence"
)

var (
	fabricstore       *FabricStore
	mockFabricstore   *FabricStore
	config            *Config
	process           string
	defaultPagination = store.Pagination{Offset: 0, Limit: 20}
	integration       = flag.Bool("integration", false, "Run integration tests")
	channelID         = flag.String("channelID", "mychannel", "channelID")
	chaincodeID       = flag.String("chaincodeID", "pop", "chaincodeID")
	configFile        = flag.String("configFile", os.Getenv("GOPATH")+"/src/github.com/stratumn/fabricstore/integration/client-config/client-config.yaml", "Absolute path to network config file")
	version           = "0.1.0"
	commit            = "00000000000000000000000000000000"
	testCtx           = context.Background()
)

func TestMain(m *testing.M) {
	flag.Parse()

	config = &Config{
		ChannelID:   *channelID,
		ChaincodeID: *chaincodeID,
		ConfigFile:  *configFile,
		Version:     version,
		Commit:      commit,
	}

	path, err := ioutil.TempDir("", "filestore")
	if err != nil {
		os.Exit(1)
	}

	evidenceStore, err := filestore.New(&filestore.Config{
		Path: path,
	})
	if err != nil {
		os.Exit(1)
	}

	mockFabricstore = NewTestClient(evidenceStore, config)

	var result int

	// Start integration network and run tests
	if *integration {
		status := make(chan bool, 1)
		go ListentNetwork(status)

		err := StartNetwork()
		if err != nil {
			StopNetwork()
			fmt.Println("Failed to start network", err.Error())
			os.Exit(1)
		}

		select {
		case success := <-status:
			if success == true {
				fmt.Println("Successfully started network, starting client")
				fabricstore, err = New(evidenceStore, config)
				if err != nil {
					fmt.Println("Could not initiate client, stopping network")
					StopNetwork()
					os.Exit(0)
				}
				fmt.Println("Client connected to fabric network, starting tests")
				time.Sleep(5 * time.Second)
				result = m.Run()
			} else {
				fmt.Println("Network didn't start successfully, stopping network")
				StopNetwork()
				os.Exit(1)
			}
		case <-time.After(time.Second * 60):
			fmt.Println("Waited network for 60 seconds, stopped network")
			StopNetwork()
			os.Exit(1)
		}

		err = StopNetwork()
		if err != nil {
			fmt.Println("Failed to stop network", err.Error())
			os.Exit(1)
		}
	} else {
		result = m.Run()
	}

	os.RemoveAll(path)
	os.Exit(result)
}

// Unit tests

func Test_AddStoreEventChannel(t *testing.T) {
	c := make(chan *store.Event, 1)
	mockFabricstore.AddStoreEventChannel(c)
}

func Test_GetInfo(t *testing.T) {
	info, err := mockFabricstore.GetInfo(testCtx)
	if err != nil {
		t.Fatalf("a.GetInfo(): err: %s", err)
	}
	if info == nil {
		t.Fatal("info = nil want interface{}")
	}
}

func Test_CreateLink(t *testing.T) {
	link := cstesting.RandomLink()
	linkHash, err := mockFabricstore.CreateLink(testCtx, link)
	assert.NoError(t, err, "CreateLink should work")
	assert.NotNil(t, linkHash, "CreateLink should return a linkHash")
}

func Test_GetSegment(t *testing.T) {
	segment := cstesting.RandomSegment()
	segment, err := mockFabricstore.GetSegment(testCtx, segment.GetLinkHash())
	assert.NoError(t, err, "GetSegment should work")
	assert.NotNil(t, segment, "GetSegment should return a segment")
}

func Test_FindSegments(t *testing.T) {
	segmentFilter := &store.SegmentFilter{Pagination: defaultPagination}
	segmentSlice, err := mockFabricstore.FindSegments(testCtx, segmentFilter)
	assert.NoError(t, err, "FindSegments should work")
	assert.NotEmpty(t, segmentSlice, "FindSegments should return several segments")
}

func Test_GetMapIDs(t *testing.T) {
	mapFilter := &store.MapFilter{}
	_, err := mockFabricstore.GetMapIDs(testCtx, mapFilter)
	if err != nil {
		t.FailNow()
	}
}

func Test_NewBatch(t *testing.T) {
	batch, err := mockFabricstore.NewBatch(testCtx)
	assert.NoError(t, err, "NewBatch should work")

	link := cstesting.RandomLink()
	linkHash, err := batch.CreateLink(testCtx, link)
	assert.NoError(t, err, "batch.CreateLink should work")
	assert.NotNil(t, linkHash, "CreateLink should return a linkHash")

	err = batch.Write(testCtx)
	assert.NoError(t, err, "batch.Write should work")
}

func Test_SetValue(t *testing.T) {
	err := mockFabricstore.SetValue(testCtx, []byte("key"), []byte("value"))
	assert.NoError(t, err, "SetValue should work")
}

func Test_GetValue(t *testing.T) {
	_, err := mockFabricstore.GetValue(testCtx, []byte("key"))
	assert.NoError(t, err, "GetValue should work")
}

func Test_DeleteValue(t *testing.T) {
	_, err := mockFabricstore.DeleteValue(testCtx, []byte("key"))
	assert.NoError(t, err, "DeleteValue should work")
}

// Integration tests (go test -integration)

func Test_SetValueIntegration(t *testing.T) {
	if !*integration {
		return
	}

	err := fabricstore.SetValue(testCtx, []byte("key"), []byte("value"))
	assert.NoError(t, err, "SetValue should work")

	value, err := fabricstore.GetValue(testCtx, []byte("key"))
	assert.NoError(t, err, "GetValue should work")
	assert.Equal(t, "value", string(value), "'value' has just been inserted below")

	value, err = fabricstore.DeleteValue(testCtx, []byte("key"))
	assert.NoError(t, err, "DeleteValue should work")
	assert.Equal(t, "value", string(value), "'value' should be returned by DeleteValue")

	value, err = fabricstore.GetValue(testCtx, []byte("key"))
	assert.NoError(t, err, "GetValue should work")
	assert.Nil(t, value, "value has be deleted below")
}

func Test_CreateLinkIntegration(t *testing.T) {
	if !*integration {
		return
	}

	link := cstesting.RandomLink()
	linkHash, err := fabricstore.CreateLink(testCtx, link)
	assert.NoError(t, err, "CreateLink should work")
	assert.NotNil(t, linkHash, "CreateLink should return a linkHash")
	lhash, _ := link.Hash()
	assert.Equal(t, lhash.String(), linkHash.String(), "CreateLink should return the created linkHash")

	segment, err := fabricstore.GetSegment(testCtx, linkHash)
	assert.NoError(t, err, "GetSegment should work")
	assert.NotNil(t, segment, "GetSegment should a segment")
}

func Test_FindSegmentsIntegration(t *testing.T) {
	if !*integration {
		return
	}

	link1 := cstesting.RandomLink()
	link2 := cstesting.RandomBranch(link1)
	link3 := cstesting.RandomLink()

	link1.Meta.PrevLinkHash = ""
	link3.Meta.PrevLinkHash = ""

	_, err := fabricstore.CreateLink(testCtx, link1)
	assert.NoError(t, err, "CreateLink(link1) should work")
	_, err = fabricstore.CreateLink(testCtx, link2)
	assert.NoError(t, err, "CreateLink(link2) should work")
	_, err = fabricstore.CreateLink(testCtx, link3)
	assert.NoError(t, err, "CreateLink(link3) should work")

	process = link1.Meta.Process

	segmentFilter := &store.SegmentFilter{
		MapIDs:     []string{link1.Meta.MapID},
		Pagination: defaultPagination,
	}

	segments, err := fabricstore.FindSegments(testCtx, segmentFilter)
	assert.NoError(t, err, "FindSegments should work")
	assert.Len(t, segments, 2, "FindSegments should find 2 segments")

	segmentFilter.MapIDs = []string{link1.Meta.MapID, link3.Meta.MapID}
	segments, err = fabricstore.FindSegments(testCtx, segmentFilter)
	assert.NoError(t, err, "FindSegments should work")
	assert.Len(t, segments, 3, "FindSegments should find 3 segments")
}

func Test_GetMapIDsIntegration(t *testing.T) {
	if !*integration {
		return
	}

	mapFilter := &store.MapFilter{
		Process:    process,
		Pagination: defaultPagination,
	}

	mapIds, err := fabricstore.GetMapIDs(testCtx, mapFilter)
	assert.NoError(t, err, "GetMapIDs should work")
	assert.NotEmpty(t, mapIds, "GetMapIDs should return an non empty list of mapIds")
}

func Test_AddStoreEventChannelIntegration(t *testing.T) {
	if !*integration {
		return
	}

	c := make(chan *store.Event)
	fabricstore.AddStoreEventChannel(c)

	link := cstesting.RandomLink()
	linkHash, _ := link.HashString()

	_, err := fabricstore.CreateLink(testCtx, link)
	assert.NoError(t, err, "CreateLink should work")

	event := <-c
	assert.Equal(t, store.SavedLinks, event.EventType, "CreateLink should send a SavedLinkEvent")
	links, ok := event.Data.([]*cs.Link)

	assert.True(t, ok, "SavedLinks should contain a []*cs.Link")
	assert.Equal(t, 1, len(links))
	retLinkHash, _ := links[0].HashString()
	assert.Equal(t, linkHash, retLinkHash, "CreateLink should send a SavedLinkEvent")
}
