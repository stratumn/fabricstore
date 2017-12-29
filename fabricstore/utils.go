package fabricstore

import (
	"errors"

	"github.com/gogo/protobuf/proto"
	fab "github.com/hyperledger/fabric-sdk-go/api/apifabclient"
	deffab "github.com/hyperledger/fabric-sdk-go/def/fabapi"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/events"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/orderer"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/utils"

	common "github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"
	com "github.com/hyperledger/fabric/protos/common"

	log "github.com/sirupsen/logrus"

	pb "github.com/hyperledger/fabric/protos/peer"
)

// Transaction is used when reading blocks
type Transaction struct {
	ChannelID     string `json:"channelID"`
	ChaincodeID   string `json:"chaincodeID"`
	TransactionID string `json:"transactionID"`
	Action        string `json:"action"`
	Args          [][]byte
}

func getChannel(client fab.FabricClient, channelID string, orgName string) (fab.Channel, error) {
	channel, err := client.NewChannel(channelID)
	if err != nil {
		return nil, err
	}

	orderers, _ := getOrderers(client)
	for _, o := range orderers {
		if err := channel.AddOrderer(o); err != nil {
			return nil, err
		}
	}

	peers, _ := getPeers(client, orgName)
	for _, p := range peers {
		if err := channel.AddPeer(p); err != nil {
			return nil, err
		}
	}

	return channel, nil
}

func getPeers(client fab.FabricClient, org string) ([]fab.Peer, error) {
	peerConfig, err := client.Config().PeersConfig(org)
	if err != nil {
		return nil, err
	}

	peers := []fab.Peer{}
	for _, p := range peerConfig {
		serverHostOverride := ""
		if str, ok := p.GRPCOptions["ssl-target-name-override"].(string); ok {
			serverHostOverride = str
		}
		peer, err := deffab.NewPeer(p.URL, p.TLSCACerts.Path, serverHostOverride, client.Config())
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

func getOrderers(client fab.FabricClient) ([]*orderer.Orderer, error) {
	ordererConfigs, err := client.Config().OrderersConfig()
	if err != nil {
		return nil, err
	}

	orderers := []*orderer.Orderer{}
	for _, o := range ordererConfigs {
		serverHostOverride := ""
		if str, ok := o.GRPCOptions["ssl-target-name-override"].(string); ok {
			serverHostOverride = str
		}
		orderer, err := orderer.NewOrderer(o.URL, o.TLSCACerts.Path, serverHostOverride, client.Config())
		if err != nil {
			return nil, err
		}
		orderers = append(orderers, orderer)
	}

	return orderers, nil
}

func getEventHub(client fab.FabricClient, orgName string) (fab.EventHub, error) {
	eventHub, err := events.NewEventHub(client)
	if err != nil {
		return nil, err
	}
	foundEventHub := false
	peerConfig, err := client.Config().PeersConfig(orgName)
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

func readBlock(block *common.Block) ([]*Transaction, error) {
	transactions := []*Transaction{}
	txFilter := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for i, txBytes := range block.Data.Data {
		if txFilter.Flag(i) == pb.TxValidationCode_VALID {
			tx, err := getChaincodeEvent(txBytes)
			if err != nil {
				return nil, err
			}
			transactions = append(transactions, tx)
		}
	}

	return transactions, nil
}

func getTxPayload(txData []byte) (*com.Payload, error) {
	if txData == nil {
		return nil, errors.New("Cannot extract payload from nil transaction")
	}

	if env, err := utils.GetEnvelopeFromBlock(txData); err != nil {
		return nil, errors.New("Error getting tx from block")
	} else if env != nil {
		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, errors.New("Could not extract payload from envelope")
		}
		return payload, nil
	}

	return nil, nil
}

func getChaincodeEvent(txData []byte) (*Transaction, error) {
	payload, err := getTxPayload(txData)
	if err != nil {
		return nil, err
	}

	chHeader, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.New("Could not extract channel header")
	}

	if com.HeaderType(chHeader.Type) == com.HeaderType_ENDORSER_TRANSACTION {
		tx, err := utils.GetTransaction(payload.Data)
		if err != nil {
			return nil, err
		}

		if len(tx.Actions) != 1 {
			return nil, errors.New("Expected exactly one item in transaction actions payload")
		}

		chaincodeActionPayload, err := utils.GetChaincodeActionPayload(tx.Actions[0].Payload)
		if err != nil {
			return nil, err
		}

		chaincodeProposalSpec := &pb.ChaincodeProposalPayload{}
		if err := proto.Unmarshal(chaincodeActionPayload.ChaincodeProposalPayload, chaincodeProposalSpec); err != nil {
			return nil, err
		}

		chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{}
		if err := proto.Unmarshal(chaincodeProposalSpec.Input, chaincodeInvocationSpec); err != nil {
			return nil, err
		}

		transaction := &Transaction{
			ChannelID:     chHeader.ChannelId,
			ChaincodeID:   chaincodeInvocationSpec.ChaincodeSpec.ChaincodeId.Name,
			TransactionID: chHeader.TxId,
			Action:        string(chaincodeInvocationSpec.ChaincodeSpec.Input.Args[0]),
			Args:          chaincodeInvocationSpec.ChaincodeSpec.Input.Args[1:],
		}

		return transaction, nil
	}

	return nil, errors.New("Not an endorser transaction")
}
