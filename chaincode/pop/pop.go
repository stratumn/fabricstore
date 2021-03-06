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

package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	sc "github.com/hyperledger/fabric/protos/peer"

	pc "github.com/stratumn/fabricstore/chaincode/pop/popconfig"

	"github.com/stratumn/go-indigocore/cs"
	"github.com/stratumn/go-indigocore/store"
)

// Pagination functionality (limit & skip) is implemented in CouchDB but not in Hyperledger Fabric (FAB-2809 and FAB-5369).
// Creating an index in CouchDB:
// curl -i -X POST -H "Content-Type: application/json" -d "{\"index\":{\"fields\":[\"chaincodeid\",\"data.docType\",\"data.id\"]},\"name\":\"indexOwner\",\"ddoc\":\"indexOwnerDoc\",\"type\":\"json\"}" http://localhost:5984/mychannel/_index

// SmartContract defines chaincode logic
type SmartContract struct {
}

// ObjectType used in CouchDB documents
const (
	ObjectTypeMap   = "map"
	ObjectTypeValue = "value"
	ObjectTypeLink  = "link"
)

// MapDoc is used to store maps in CouchDB
type MapDoc struct {
	ObjectType string `json:"docType"`
	ID         string `json:"id"`
	Process    string `json:"process"`
}

// LinkDoc is used to store links in CouchDB
type LinkDoc struct {
	ObjectType string   `json:"docType"`
	ID         string   `json:"id"`
	Link       *cs.Link `json:"link"`
}

// MapSelector used in MapQuery
type MapSelector struct {
	ObjectType string `json:"docType"`
	Process    string `json:"process,omitempty"`
}

// MapQuery used in CouchDB rich queries
type MapQuery struct {
	Selector MapSelector `json:"selector,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	Skip     int         `json:"skip,omitempty"`
}

func newMapQuery(filterBytes []byte) (string, error) {
	filter := &store.MapFilter{}
	if err := json.Unmarshal(filterBytes, filter); err != nil {
		return "", err
	}

	mapSelector := MapSelector{}
	mapSelector.ObjectType = ObjectTypeMap

	if filter.Process != "" {
		mapSelector.Process = filter.Process
	}

	mapQuery := MapQuery{
		Selector: mapSelector,
		Limit:    filter.Pagination.Limit,
		Skip:     filter.Pagination.Offset,
	}

	queryBytes, err := json.Marshal(mapQuery)
	if err != nil {
		return "", err
	}

	return string(queryBytes), nil
}

// LinkSelector used in LinkQuery
type LinkSelector struct {
	ObjectType   string    `json:"docType"`
	LinkHash     string    `json:"id,omitempty"`
	PrevLinkHash string    `json:"link.meta.prevLinkHash,omitempty"`
	Process      string    `json:"link.meta.process,omitempty"`
	MapIds       *MapIdsIn `json:"link.meta.mapId,omitempty"`
	Tags         *TagsAll  `json:"link.meta.tags,omitempty"`
}

// MapIdsIn specifies that segment mapId should be in specified list
type MapIdsIn struct {
	MapIds []string `json:"$in,omitempty"`
}

// TagsAll specifies all tags in specified list should be in segment tags
type TagsAll struct {
	Tags []string `json:"$all,omitempty"`
}

// LinkQuery used in CouchDB rich queries
type LinkQuery struct {
	Selector LinkSelector `json:"selector,omitempty"`
	Limit    int          `json:"limit,omitempty"`
	Skip     int          `json:"skip,omitempty"`
}

func newLinkQuery(filterBytes []byte) (string, error) {
	filter := &store.SegmentFilter{}
	if err := json.Unmarshal(filterBytes, filter); err != nil {
		return "", err
	}

	linkSelector := LinkSelector{}
	linkSelector.ObjectType = ObjectTypeLink

	if filter.PrevLinkHash != nil {
		linkSelector.PrevLinkHash = *filter.PrevLinkHash
	}
	if filter.Process != "" {
		linkSelector.Process = filter.Process
	}
	if len(filter.MapIDs) > 0 {
		linkSelector.MapIds = &MapIdsIn{filter.MapIDs}
	} else {
		linkSelector.Tags = nil
	}
	if len(filter.Tags) > 0 {
		linkSelector.Tags = &TagsAll{filter.Tags}
	} else {
		linkSelector.Tags = nil
	}

	linkQuery := LinkQuery{
		Selector: linkSelector,
		Limit:    filter.Pagination.Limit,
		Skip:     filter.Pagination.Offset,
	}

	queryBytes, err := json.Marshal(linkQuery)
	if err != nil {
		return "", err
	}

	return string(queryBytes), nil
}

// Init method is called when the Smart Contract "pop" is instantiated by the blockchain network
func (s *SmartContract) Init(APIstub shim.ChaincodeStubInterface) sc.Response {
	return shim.Success(nil)
}

// Invoke method is called as a result of an application request to run the Smart Contract "pop"
func (s *SmartContract) Invoke(APIstub shim.ChaincodeStubInterface) sc.Response {
	// Retrieve the requested Smart Contract function and arguments
	function, args := APIstub.GetFunctionAndParameters()

	switch function {
	case pc.GetLink:
		return s.GetLink(APIstub, args)
	case pc.CreateLink:
		return s.CreateLink(APIstub, args)
	case pc.DeleteLink:
		return s.DeleteLink(APIstub, args)
	case pc.FindLinks:
		return s.FindLinks(APIstub, args)
	case pc.GetMapIDs:
		return s.GetMapIDs(APIstub, args)
	case pc.SetValue:
		return s.SetValue(APIstub, args)
	case pc.GetValue:
		return s.GetValue(APIstub, args)
	case pc.DeleteValue:
		return s.DeleteValue(APIstub, args)
	default:
		return shim.Error("Invalid Smart Contract function name: " + function)
	}
}

// saveMap saves map into CouchDB using map document
func (s *SmartContract) saveMap(stub shim.ChaincodeStubInterface, link *cs.Link) error {
	mapDoc := MapDoc{
		ObjectType: ObjectTypeMap,
		ID:         link.Meta.MapID,
		Process:    link.Meta.Process,
	}
	mapDocBytes, err := json.Marshal(mapDoc)
	if err != nil {
		return err
	}

	return stub.PutState(mapDoc.ID, mapDocBytes)
}

// CreateLink persists Link to blockchain.
func (s *SmartContract) CreateLink(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	byteArgs := stub.GetArgs()
	link := &cs.Link{}
	if err := json.Unmarshal(byteArgs[1], link); err != nil {
		return shim.Error("Could not parse link")
	}

	// Check has prevLinkHash if not create map else check prevLinkHash exists
	prevLinkHash := link.Meta.PrevLinkHash
	if prevLinkHash == "" {
		if err := s.saveMap(stub, link); err != nil {
			return shim.Error(err.Error())
		}
	}

	linkHashString, err := link.HashString()
	if err != nil {
		return shim.Error(err.Error())
	}

	linkDoc := &LinkDoc{
		ObjectType: ObjectTypeLink,
		ID:         linkHashString,
		Link:       link,
	}
	linkDocBytes, err := json.Marshal(linkDoc)
	if err != nil {
		return shim.Error(err.Error())
	}

	if err := stub.PutState(linkDoc.ID, linkDocBytes); err != nil {
		return shim.Error(err.Error())
	}

	// Send event
	if err := stub.SetEvent(pc.CreateLink, byteArgs[1]); err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// GetLink gets Link for given linkHash
func (s *SmartContract) GetLink(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	linkDocBytes, err := stub.GetState(args[0])
	if err != nil {
		return shim.Error(err.Error())
	}
	if linkDocBytes == nil {
		return shim.Success(nil)
	}

	linkBytes, err := extractLink(linkDocBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(linkBytes)
}

// DeleteLink deletes link from CouchDB
func (s *SmartContract) DeleteLink(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	shimResponse := s.GetLink(stub, args)
	if shimResponse.Status == shim.ERROR {
		return shimResponse
	}
	linkBytes := shimResponse.Payload
	err := stub.DelState(args[0])
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(linkBytes)
}

// FindLinks returns segments that match specified segment filter
func (s *SmartContract) FindLinks(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	queryString, err := newLinkQuery([]byte(args[0]))
	if err != nil {
		return shim.Error("Segment filter format incorrect")
	}

	resultsIterator, err := stub.GetQueryResult(queryString)
	if err != nil {
		return shim.Error(err.Error())
	}

	var links []*cs.Link

	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return shim.Error(err.Error())
		}
		linkDoc := &LinkDoc{}
		if err := json.Unmarshal(queryResponse.Value, linkDoc); err != nil {
			return shim.Error(err.Error())
		}
		links = append(links, linkDoc.Link)
	}

	resultBytes, err := json.Marshal(links)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(resultBytes)
}

// GetMapIDs returns mapIDs for maps that match specified map filter
func (s *SmartContract) GetMapIDs(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	queryString, err := newMapQuery([]byte(args[0]))
	if err != nil {
		return shim.Error("Map filter format incorrect")
	}

	resultsIterator, err := stub.GetQueryResult(queryString)
	if err != nil {
		return shim.Error(err.Error())
	}

	var mapIDs []string
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return shim.Error(err.Error())
		}
		mapIDs = append(mapIDs, queryResponse.Key)
	}

	sort.Strings(mapIDs)
	resultBytes, err := json.Marshal(mapIDs)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(resultBytes)
}

// SetValue saves key, value in CouchDB
func (s *SmartContract) SetValue(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	compositeKey, err := getValueCompositeKey(args[0], stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	err = stub.PutState(compositeKey, []byte(args[1]))
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(nil)
}

// GetValue gets value for specified key from CouchDB
func (s *SmartContract) GetValue(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	compositeKey, err := getValueCompositeKey(args[0], stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	value, err := stub.GetState(compositeKey)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(value)
}

// DeleteValue deletes key, value from CouchDB
func (s *SmartContract) DeleteValue(stub shim.ChaincodeStubInterface, args []string) sc.Response {
	compositeKey, err := getValueCompositeKey(args[0], stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	value, err := stub.GetState(compositeKey)
	if err != nil {
		return shim.Error(err.Error())
	}

	err = stub.DelState(compositeKey)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(value)
}

func extractLink(linkDocBytes []byte) ([]byte, error) {
	linkDoc := &LinkDoc{}
	if err := json.Unmarshal(linkDocBytes, linkDoc); err != nil {
		return nil, err
	}
	linkBytes, err := json.Marshal(linkDoc.Link)
	if err != nil {
		return nil, err
	}
	return linkBytes, nil
}

func getValueCompositeKey(key string, stub shim.ChaincodeStubInterface) (compositeKey string, err error) {
	compositeKey, err = stub.CreateCompositeKey(ObjectTypeValue, []string{key})
	return
}

// main function starts up the chaincode in the container during instantiate
func main() {
	if err := shim.Start(new(SmartContract)); err != nil {
		fmt.Printf("Error starting SmartContract chaincode: %s", err)
	}
}
