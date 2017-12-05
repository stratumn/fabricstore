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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stratumn/sdk/cs"
	"github.com/stratumn/sdk/cs/cstesting"
	"github.com/stratumn/sdk/filestore"
	"github.com/stratumn/sdk/store"
	"github.com/stratumn/sdk/types"

	_ "github.com/stratumn/fabricstore/evidence"
)

var (
	err             error
	fabricstore     *FabricStore
	mockFabricstore *FabricStore
	test            *testing.T
	linkHash        *types.Bytes32
	config          *Config
	process         string
	integration     = flag.Bool("integration", false, "Run integration tests")
	channelID       = flag.String("channelID", "mychannel", "channelID")
	chaincodeID     = flag.String("chaincodeID", "pop", "chaincodeID")
	configFile      = flag.String("configFile", os.Getenv("GOPATH")+"/src/github.com/stratumn/fabricstore/integration/client-config/client-config.yaml", "Absolute path to network config file")
	version         = "0.1.0"
	commit          = "00000000000000000000000000000000"
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

func Test_AddDidSaveChannel(t *testing.T) {
	c := make(chan *cs.Segment, 1)
	mockFabricstore.AddDidSaveChannel(c)
}

func Test_GetInfo(t *testing.T) {
	info, err := mockFabricstore.GetInfo()
	if err != nil {
		t.Fatalf("a.GetInfo(): err: %s", err)
	}
	if info == nil {
		t.Fatal("info = nil want interface{}")
	}
}

func Test_SaveSegment(t *testing.T) {
	segment := cstesting.RandomSegment()
	err := mockFabricstore.SaveSegment(segment)
	if err != nil {
		t.FailNow()
	}
}

func Test_GetSegment(t *testing.T) {
	segment := cstesting.RandomSegment()
	_, err := mockFabricstore.GetSegment(segment.GetLinkHash())
	if err != nil {
		t.Logf(err.Error())
		t.FailNow()
	}
}

func Test_DeleteSegment(t *testing.T) {
	segment := cstesting.RandomSegment()
	_, err := mockFabricstore.DeleteSegment(segment.GetLinkHash())
	if err != nil {
		t.Logf(err.Error())
		t.FailNow()
	}
}

func Test_FindSegments(t *testing.T) {
	segmentFilter := &store.SegmentFilter{}
	_, err := mockFabricstore.FindSegments(segmentFilter)
	if err != nil {
		t.FailNow()
	}
}

func Test_GetMapIDs(t *testing.T) {
	mapFilter := &store.MapFilter{}
	_, err := mockFabricstore.GetMapIDs(mapFilter)
	if err != nil {
		t.FailNow()
	}
}

func Test_NewBatch(t *testing.T) {
	batch, err := mockFabricstore.NewBatch()
	if err != nil {
		t.FailNow()
	}
	batch.SaveValue([]byte("key"), []byte("value"))

	segment := cstesting.RandomSegment()
	batch.SaveSegment(segment)

	batch.Write()

	batch.DeleteValue([]byte("key"))
	batch.DeleteSegment(segment.GetLinkHash())

	batch.Write()
}

func Test_SaveValue(t *testing.T) {
	err := mockFabricstore.SaveValue([]byte("key"), []byte("value"))
	if err != nil {
		t.FailNow()
	}
}

func Test_GetValue(t *testing.T) {
	_, err := mockFabricstore.GetValue([]byte("key"))
	if err != nil {
		t.FailNow()
	}
}

func Test_DeleteValue(t *testing.T) {
	_, err := mockFabricstore.DeleteValue([]byte("key"))
	if err != nil {
		t.FailNow()
	}
}

// Integration tests (go test -integration)

func Test_SaveValueIntegration(t *testing.T) {
	if *integration {
		err := fabricstore.SaveValue([]byte("key"), []byte("value"))
		if err != nil {
			fmt.Println("Could not save value", err.Error())
			t.FailNow()
		}
	}
}

func Test_GetValueIntegration(t *testing.T) {
	if *integration {
		value, err := fabricstore.GetValue([]byte("key"))
		if err != nil {
			fmt.Println("Could not get saved value", err.Error())
			t.FailNow()
		}
		if string(value) != "value" {
			fmt.Println("Unexpected value in store")
			t.FailNow()
		}
	}
}

func Test_DeleteValueIntegration(t *testing.T) {
	if *integration {
		_, err := fabricstore.DeleteValue([]byte("key"))
		if err != nil {
			fmt.Println("Error while deleting value", err.Error())
			t.FailNow()
		}
	}
}

func Test_GetValueMissingIntegration(t *testing.T) {
	if *integration {
		value, err := fabricstore.GetValue([]byte("key"))
		if err != nil {
			fmt.Println("Did not expect error getting non existent key", err.Error())
			t.FailNow()
		}
		if value != nil {
			fmt.Println("Value:", value, "not deleted")
			t.FailNow()
		}
	}
}

func Test_SaveSegmentIntegration(t *testing.T) {
	if *integration {
		segment := cstesting.RandomSegment()
		linkHash = segment.GetLinkHash()
		err := fabricstore.SaveSegment(segment)
		if err != nil {
			fmt.Println("Could not save segment", err.Error())
			t.FailNow()
		}
	}
}

func Test_GetSegmentIntegration(t *testing.T) {
	if *integration {
		segment, err := fabricstore.GetSegment(linkHash)
		if err != nil {
			fmt.Println("Could not get segment", err.Error())
			t.FailNow()
		}

		if segment.GetLinkHashString() != linkHash.String() {
			fmt.Println("Did not retrieve expected segment")
			t.FailNow()
		}
	}
}

func Test_DeleteSegmentIntegration(t *testing.T) {
	if *integration {
		_, err = fabricstore.DeleteSegment(linkHash)
		if err != nil {
			fmt.Println("Could not delete segment", err.Error())
			t.FailNow()
		}
	}
}

func Test_GetSegmentMissingIntegration(t *testing.T) {
	if *integration {
		segment, err := fabricstore.GetSegment(linkHash)
		if err != nil {
			fmt.Println("Could not get segment", err.Error())
			t.FailNow()
		}

		if segment != nil {
			fmt.Println("Expected nil segment")
			t.FailNow()
		}
	}
}

func Test_FindSegmentsIntegration(t *testing.T) {
	if *integration {
		segment1 := cstesting.RandomSegment()
		segment2 := cstesting.RandomBranch(segment1)
		segment3 := cstesting.RandomSegment()

		delete(segment1.Link.Meta, "prevLinkHash")
		delete(segment3.Link.Meta, "prevLinkHash")

		err := fabricstore.SaveSegment(segment1)
		err = fabricstore.SaveSegment(segment2)
		err = fabricstore.SaveSegment(segment3)

		if err != nil {
			fmt.Println("Could not save segments", err.Error())
		}

		process = segment1.Link.GetProcess()

		segmentFilter := &store.SegmentFilter{
			MapIDs: []string{segment1.Link.GetMapID()},
			Pagination: store.Pagination{
				Limit: 20,
			},
		}

		segments, err := fabricstore.FindSegments(segmentFilter)
		if err != nil {
			fmt.Println("Could not find segments", err.Error())
			t.FailNow()
		}
		if len(segments) != 2 {
			fmt.Println("Expected 2 segments got", len(segments))
			t.FailNow()
		}

		segmentFilter.MapIDs = []string{segment1.Link.GetMapID(), segment3.Link.GetMapID()}
		segments, err = fabricstore.FindSegments(segmentFilter)
		if err != nil {
			fmt.Println("Could not find segments", err.Error())
			t.FailNow()
		}
		if len(segments) != 3 {
			fmt.Println("Expected 3 segments got", len(segments))
			t.FailNow()
		}
	}
}

func Test_GetMapIDsIntegration(t *testing.T) {
	if *integration {
		mapFilter := &store.MapFilter{
			Process: process,
			Pagination: store.Pagination{
				Limit: 20,
			},
		}

		mapIds, err := fabricstore.GetMapIDs(mapFilter)

		if err != nil {
			fmt.Println("Could not find mapIds", err.Error())
			t.FailNow()
		}
		if len(mapIds) == 0 {
			fmt.Println("Expected at least one mapId")
			t.FailNow()
		}
	}
}

func Test_AddDidSaveChannelIntegration(t *testing.T) {
	if *integration {
		c := make(chan *cs.Segment, 1)
		fabricstore.AddDidSaveChannel(c)

		segment := cstesting.RandomSegment()
		if err := fabricstore.SaveSegment(segment); err != nil {
			t.FailNow()
		}

		if got, want := <-c, segment; want.GetLinkHashString() != got.GetLinkHashString() {
			t.Errorf("Didn't receive segment via didSaveChan")
			t.FailNow()
		}
	}
}
