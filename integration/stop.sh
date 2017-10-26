docker-compose -f $GOPATH/src/github.com/stratumn/fabricstore/integration/docker-compose.yml down --remove-orphans
docker rm -f $(docker ps -aq)
docker rmi $(docker images | grep dev-peer0.org1.example.com-pop-1.0 | awk "{print \$3}")
rm -rf $GOPATH/src/github.com/stratumn/fabricstore/cmd/msp
rm -rf $GOPATH/src/github.com/stratumn/fabricstore/cmd/keystore
rm -rf $GOPATH/src/github.com/stratumn/fabricstore/store/msp
rm -rf $GOPATH/src/github.com/stratumn/fabricstore/store/keystore
