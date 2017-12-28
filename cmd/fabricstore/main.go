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

// The command fabricstore starts an HTTP server with a fabricstore.

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stratumn/fabricstore/fabricstore"
	"github.com/stratumn/sdk/filestore"
	"github.com/stratumn/sdk/store/storehttp"

	"github.com/stratumn/fabricstore/util"
)

var (
	channelID   = flag.String("channelID", os.Getenv("CHANNEL_ID"), "channelID")
	chaincodeID = flag.String("chaincodeID", os.Getenv("CHAINCODE_ID"), "chaincodeID")
	configFile  = flag.String("configFile", os.Getenv("CLIENT_CONFIG_PATH"), "Absolute path to network config file")
	path        = flag.String("path", os.Getenv("LOCALSTORAGE_PATH"), "Path used for local storage")
	version     = "0.1.0"
	commit      = "00000000000000000000000000000000"
)

func init() {
	storehttp.RegisterFlags()
}

func main() {
	flag.Parse()
	log.Infof("%s v%s@%s", fabricstore.Description, version, commit[:7])

	localPath := *path

	if localPath == "" {
		var err error

		localPath, err = ioutil.TempDir("", "filestore")
		if err != nil {
			log.Fatalf("Could not create local temp file for local storage: %v", err)
		}
	}

	evidenceStore, err := filestore.New(&filestore.Config{
		Path: *path,
	})
	if err != nil {
		log.Fatalf("Could not start local storage: %v", err)
	}

	var a *fabricstore.FabricStore
	var storeErr error

	err = util.Retry(func(attempt int) (retry bool, err error) {
		a, storeErr = fabricstore.New(evidenceStore, &fabricstore.Config{
			ChannelID:   *channelID,
			ChaincodeID: *chaincodeID,
			ConfigFile:  *configFile,
			Version:     version,
			Commit:      commit,
		})

		if storeErr != nil {
			log.Infof("Unable to connect to fabric network (%v). Retrying in 5s.", storeErr.Error())
			time.Sleep(5 * time.Second)
			return true, storeErr
		}

		return false, storeErr
	}, 12)

	if err != nil {
		log.Fatalf("Could not start fabric client: %v", err)
	}

	storehttp.RunWithFlags(a)
}
