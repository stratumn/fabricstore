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
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/stratumn/fabricstore/fabricstore"
	"github.com/stratumn/sdk/store/storehttp"
)

var (
	channelID   = flag.String("channelID", "mychannel", "channelID")
	chaincodeID = flag.String("chaincodeID", "pop", "chaincodeID")
	configFile  = flag.String("configFile", os.Getenv("GOPATH")+"/src/github.com/stratumn/fabricstore/integration/client-config/client-config.yaml", "Absolute path to network config file")
	version     = "0.1.0"
	commit      = "00000000000000000000000000000000"
)

func init() {
	storehttp.RegisterFlags()
}

func main() {
	flag.Parse()
	log.Infof("%s v%s@%s", fabricstore.Description, version, commit[:7])

	a, err := fabricstore.New(&fabricstore.Config{
		ChannelID:   *channelID,
		ChaincodeID: *chaincodeID,
		ConfigFile:  *configFile,
		Version:     version,
		Commit:      commit,
	})
	if err != nil {
		log.Fatalf("Could not start fabric client: %v", err)
	}

	storehttp.RunWithFlags(a)
}
