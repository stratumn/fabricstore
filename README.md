# Stratumn Fabric Store

fabricstore implements stratumn/sdk/store interface and allow to deploy an Indigo node using a Hyperledger Fabric back end.

### Deployment

fabricstore requires 3 environment variables:
* CHANNEL_ID: channel id used on the Hyperledger Fabric network to deploy the chaincode
* CHAINCODE_ID: chaincode id used to deploy indigo proof of process on the Hyperledger Fabric network
* CLIENT_CONFIG_PATH: absolute path to the client configuration file (example under integration/client-config/client-config.yaml)

### Testing

To test fabricstore first install dependencies using `dep ensure`.

Then run `make test`.

This will launch a basic Hyperledger Fabric network with one orderer (solo consensus) and one peer using couchdb for its world state.

### License

Copyright 2017 Stratumn SAS. All rights reserved.

Unless otherwise noted, the source files are distributed under the Apache
License 2.0 found in the LICENSE file.

Third party dependencies included in the vendor directory are distributed under
their respective licenses.
