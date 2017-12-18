# Stratumn Fabricstore

fabricstore implements [stratumn/sdk/store.Adapter](https://github.com/stratumn/sdk/blob/master/store/store.go) interface and allows the deployment of an [Indigo](https://indigoframework.com) node using a Hyperledger Fabric back end.

### Deployment

fabricstore requires 4 environment variables:
* `CHANNEL_ID`: channel id used on the Hyperledger Fabric network to deploy the chaincode
* `CHAINCODE_ID`: chaincode id used to deploy indigo proof of process on the Hyperledger Fabric network
* `CLIENT_CONFIG_PATH`: absolute path to the client configuration file (example under `integration/client-config/client-config.yaml`)
* `LOCALSTORAGE_PATH`: absolute path to a local storage folder

### Testing

To test fabricstore a basic Hyperledger Fabric network is launched with only one orderer (solo consensus) and one peer using couchdb for storage.

The network configuration is detailed in `integration/configtx.yaml` and `integration/crypto-config.yaml`. For convenience network artifacts and certificates have already been generated from these two config files. However you could regenerate the network artifacts from these files using Fabric's cryptogen and configtxgen tools ([hyperledger-fabric.readthedocs.io](https://hyperledger-fabric.readthedocs.io)). If you change the integration test network you should update `integration/client-config/client-config.yaml` accordingly.

---

To test fabricstore first install dependencies using `dep ensure`.

Then run `make test`.

This will download the required Fabric images and launch the network using docker compose.

---

If you want to start/stop the integration network outside the test you can do so by running `integration/start.sh` and `integration/stop.sh`.

### License

Copyright 2017 Stratumn SAS. All rights reserved.

Unless otherwise noted, the source files are distributed under the Apache
License 2.0 found in the LICENSE file.

Third party dependencies included in the vendor directory are distributed under
their respective licenses.
