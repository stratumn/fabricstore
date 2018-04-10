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
	"errors"
	"flag"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"

	pc "github.com/stratumn/fabricstore/chaincode/pop/popconfig"

	"github.com/stratumn/go-indigocore/cs"
	"github.com/stratumn/go-indigocore/cs/cstesting"
	"github.com/stratumn/go-indigocore/store"
	"github.com/stratumn/go-indigocore/testutil"
)

var (
	integration = flag.Bool("integration", false, "Run integration tests")
)

func checkQuery(t *testing.T, stub *shim.MockStub, args [][]byte) []byte {
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		fmt.Println("Query failed", string(res.Message))
		t.FailNow()
	}
	if res.Payload == nil {
		fmt.Println("Query failed to get value")
		t.FailNow()
	}

	return res.Payload
}

func checkInvoke(t *testing.T, stub *shim.MockStub, args [][]byte) []byte {
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		fmt.Println("Invoke", string(args[0]), "failed", string(res.Message))
		t.FailNow()
	}
	return res.Payload
}

func createLink(t *testing.T, stub *shim.MockStub, link *cs.Link) {
	linkBytes, err := json.Marshal(link)
	if err != nil {
		fmt.Println("Could not marshal link")
	}

	checkInvoke(t, stub, [][]byte{[]byte(pc.CreateLink), linkBytes})
}

func TestPop_Init(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	stub.MockInit("1", [][]byte{[]byte("init")})
}

func TestPop_InvalidFunction(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	stub.MockInvoke("1", [][]byte{[]byte("0000")})
}

func TestPop_CreateLink(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	link := cstesting.RandomLink()
	link.Meta.PrevLinkHash = ""
	linkHashString, err := link.HashString()
	if err != nil {
		t.Logf(err.Error())
		t.FailNow()
	}

	createLink(t, stub, link)

	payload := checkQuery(t, stub, [][]byte{[]byte(pc.GetLink), []byte(linkHashString)})
	savedLink := &cs.Link{}
	json.Unmarshal(payload, savedLink)

	linkBytes, _ := json.Marshal(link)

	if string(linkBytes) != string(payload) {
		fmt.Println("Link not saved into database")
		t.FailNow()
	}

	checkInvoke(t, stub, [][]byte{[]byte(pc.DeleteLink), []byte(linkHashString)})
	res := stub.MockInvoke("1", [][]byte{[]byte(pc.GetLink), []byte(linkHashString)})
	if res.Payload != nil {
		fmt.Println("DeleteLink failed")
		t.FailNow()
	}
}

func TestPop_FindLinksMock(t *testing.T) {
	contract := SmartContract{}
	stub := &FindLinksMockStub{}
	filter := store.SegmentFilter{}
	filterBytes, _ := json.Marshal(filter)
	contract.FindLinks(stub, []string{string(filterBytes)})
}

func TestPop_FindLinks(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	filter := store.SegmentFilter{}
	filterBytes, _ := json.Marshal(filter)

	res := stub.MockInvoke("1", [][]byte{[]byte(pc.FindLinks), filterBytes})
	if res.Status == shim.ERROR {
		if string(res.Message) != "Not Implemented" {
			t.FailNow()
		}
	}
}

func TestPop_GetMapIDsMock(t *testing.T) {
	contract := SmartContract{}
	stub := &GetMapIDsMockStub{}
	filter := store.MapFilter{}
	filterBytes, _ := json.Marshal(filter)
	contract.GetMapIDs(stub, []string{string(filterBytes)})
}

func TestPop_GetMapIDs(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	filter := store.MapFilter{}
	filterBytes, _ := json.Marshal(filter)

	res := stub.MockInvoke("1", [][]byte{[]byte(pc.GetMapIDs), filterBytes})
	if res.Status == shim.ERROR {
		if string(res.Message) != "Not Implemented" {
			t.FailNow()
		}
	}
}

func TestPop_GetLinkDoesNotExist(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	res := stub.MockInvoke("1", [][]byte{[]byte(pc.GetLink), []byte("")})
	if res.Payload != nil {
		fmt.Println("GetLink should return nil")
		t.FailNow()
	}
}

func TestPop_SetValue(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	checkInvoke(t, stub, [][]byte{[]byte(pc.SetValue), []byte("key"), []byte("value")})

	payload := checkQuery(t, stub, [][]byte{[]byte(pc.GetValue), []byte("key")})
	if string(payload) != "value" {
		fmt.Println("Could not find set value")
		t.FailNow()
	}

	checkInvoke(t, stub, [][]byte{[]byte(pc.DeleteValue), []byte("key")})
	res := stub.MockInvoke("1", [][]byte{[]byte(pc.GetValue), []byte("key")})
	if res.Payload != nil {
		fmt.Println("DeleteValue failed")
		t.FailNow()
	}
}

func TestPop_newMapQuery(t *testing.T) {
	pagination := store.Pagination{
		Limit:  10,
		Offset: 15,
	}
	mapFilter := &store.MapFilter{
		Process:    "main",
		Pagination: pagination,
	}

	filterBytes, err := json.Marshal(mapFilter)
	if err != nil {
		t.FailNow()
	}
	queryString, err := newMapQuery(filterBytes)
	if queryString != "{\"selector\":{\"docType\":\"map\",\"process\":\"main\"},\"limit\":10,\"skip\":15}" {
		fmt.Println("Map query failed")
		t.FailNow()
	}
}

func TestPop_newLinkQuery(t *testing.T) {
	pagination := store.Pagination{
		Limit:  10,
		Offset: 15,
	}

	linkHash := "085fa4322980286778f896fe11c4f55c46609574d9188a3c96427c76b8500bcd"

	segmentFilter := &store.SegmentFilter{
		MapIDs:       []string{"map1", "map2"},
		Process:      "main",
		PrevLinkHash: &linkHash,
		Tags:         []string{"tag1"},
		Pagination:   pagination,
	}
	filterBytes, err := json.Marshal(segmentFilter)
	if err != nil {
		t.FailNow()
	}
	queryString, err := newLinkQuery(filterBytes)
	if queryString != "{\"selector\":{\"docType\":\"link\",\"link.meta.prevLinkHash\":\"085fa4322980286778f896fe11c4f55c46609574d9188a3c96427c76b8500bcd\",\"link.meta.process\":\"main\",\"link.meta.mapId\":{\"$in\":[\"map1\",\"map2\"]},\"link.meta.tags\":{\"$all\":[\"tag1\"]}},\"limit\":10,\"skip\":15}" {
		fmt.Println("Link query failed")
		t.FailNow()
	}
}

type GetMapIDsMockStub struct {
	shim.MockStub
}

func (p *GetMapIDsMockStub) GetQueryResult(querystring string) (shim.StateQueryIteratorInterface, error) {
	mapDoc := &MapDoc{
		ObjectType: ObjectTypeMap,
		ID:         testutil.RandomString(24),
		Process:    "main",
	}
	iterator := mapIterator{
		[]*MapDoc{mapDoc},
	}
	return &iterator, nil
}

type mapIterator struct {
	MapDocs []*MapDoc
}

func (m *mapIterator) HasNext() bool {
	if len(m.MapDocs) > 0 {
		return true
	}
	return false
}

func (m *mapIterator) Next() (*queryresult.KV, error) {
	if len(m.MapDocs) == 0 {
		return nil, errors.New("Empty")
	}
	mapDoc := m.MapDocs[0]
	m.MapDocs = m.MapDocs[1:]

	mapDocBytes, _ := json.Marshal(mapDoc)

	return &queryresult.KV{
		Key:       mapDoc.ID,
		Value:     mapDocBytes,
		Namespace: "mychannel",
	}, nil
}

func (m *mapIterator) Close() error {
	return nil

}

type FindLinksMockStub struct {
	shim.MockStub
}

func (p *FindLinksMockStub) GetQueryResult(queryString string) (shim.StateQueryIteratorInterface, error) {
	segment := cstesting.RandomSegment()
	linkDoc := &LinkDoc{
		ObjectType: ObjectTypeLink,
		ID:         segment.GetLinkHashString(),
		Link:       &segment.Link,
	}
	iterator := linkIterator{
		[]*LinkDoc{linkDoc},
	}
	return &iterator, nil
}

type linkIterator struct {
	LinkDocs []*LinkDoc
}

func (s *linkIterator) HasNext() bool {
	if len(s.LinkDocs) > 0 {
		return true
	}
	return false
}
func (s *linkIterator) Next() (*queryresult.KV, error) {
	if len(s.LinkDocs) == 0 {
		return nil, errors.New("Empty")
	}
	linkDoc := s.LinkDocs[0]
	linkDocBytes, err := json.Marshal(linkDoc)
	if err != nil {
		return nil, err
	}

	s.LinkDocs = s.LinkDocs[1:]

	return &queryresult.KV{
		Key:       linkDoc.ID,
		Value:     linkDocBytes,
		Namespace: "mychannel",
	}, nil
}

func (s *linkIterator) Close() error {
	return nil
}
