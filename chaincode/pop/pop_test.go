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

	"github.com/stratumn/sdk/cs"
	"github.com/stratumn/sdk/cs/cstesting"
	"github.com/stratumn/sdk/store"
	"github.com/stratumn/sdk/testutil"
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

func saveSegment(t *testing.T, stub *shim.MockStub, segment *cs.Segment) {
	segmentBytes, err := json.Marshal(segment)
	if err != nil {
		fmt.Println("Could not marshal segment")
	}

	checkInvoke(t, stub, [][]byte{[]byte("SaveSegment"), segmentBytes})
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

func TestPop_SaveSegment(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	segment := cstesting.RandomSegment()
	delete(segment.Link.Meta, "prevLinkHash")

	saveSegment(t, stub, segment)

	payload := checkQuery(t, stub, [][]byte{[]byte("GetSegment"), []byte(segment.GetLinkHashString())})
	savedSegment := &cs.Segment{}
	json.Unmarshal(payload, savedSegment)

	segment.Meta.Evidences = savedSegment.Meta.Evidences
	segmentBytes, _ := json.Marshal(segment)

	if string(segmentBytes) != string(payload) {
		fmt.Println("Segment not saved into database")
		t.FailNow()
	}

	checkInvoke(t, stub, [][]byte{[]byte("DeleteSegment"), []byte(segment.GetLinkHashString())})
	res := stub.MockInvoke("1", [][]byte{[]byte("GetSegment"), []byte(segment.GetLinkHashString())})
	if res.Payload != nil {
		fmt.Println("DeleteSegment failed")
		t.FailNow()
	}
}

func TestPop_FindSegmentsMock(t *testing.T) {
	contract := SmartContract{}
	stub := &FindSegmentsMockStub{}
	filter := store.SegmentFilter{}
	filterBytes, _ := json.Marshal(filter)
	contract.FindSegments(stub, []string{string(filterBytes)})
}

func TestPop_FindSegments(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	filter := store.SegmentFilter{}
	filterBytes, _ := json.Marshal(filter)

	res := stub.MockInvoke("1", [][]byte{[]byte("FindSegments"), filterBytes})
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

	res := stub.MockInvoke("1", [][]byte{[]byte("GetMapIDs"), filterBytes})
	if res.Status == shim.ERROR {
		if string(res.Message) != "Not Implemented" {
			t.FailNow()
		}
	}
}

func TestPop_SaveSegmentIncorrect(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	res := stub.MockInvoke("1", [][]byte{[]byte("SaveSegment"), []byte("")})
	if res.Status != shim.ERROR {
		fmt.Println("SaveSegment should have failed")
		t.FailNow()
	} else {
		if res.Message != "Could not parse segment" {
			fmt.Println("Failed with error", res.Message, "expected", "Could not parse segment")
			t.FailNow()
		}
	}
}

func TestPop_GetSegmentDoesNotExist(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	res := stub.MockInvoke("1", [][]byte{[]byte("GetSegment"), []byte("")})
	if res.Payload != nil {
		fmt.Println("GetSegment should have failed")
		t.FailNow()
	}
}

func TestPop_SaveValue(t *testing.T) {
	cc := new(SmartContract)
	stub := shim.NewMockStub("pop", cc)

	checkInvoke(t, stub, [][]byte{[]byte("SaveValue"), []byte("key"), []byte("value")})

	payload := checkQuery(t, stub, [][]byte{[]byte("GetValue"), []byte("key")})
	if string(payload) != "value" {
		fmt.Println("Could not find saved value")
		t.FailNow()
	}

	checkInvoke(t, stub, [][]byte{[]byte("DeleteValue"), []byte("key")})
	res := stub.MockInvoke("1", [][]byte{[]byte("GetValue"), []byte("key")})
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

func TestPop_newSegmentQuery(t *testing.T) {
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
	queryString, err := newSegmentQuery(filterBytes)
	if queryString != "{\"selector\":{\"docType\":\"segment\",\"segment.link.meta.prevLinkHash\":\"085fa4322980286778f896fe11c4f55c46609574d9188a3c96427c76b8500bcd\",\"segment.link.meta.process\":\"main\",\"segment.link.meta.mapId\":{\"$in\":[\"map1\",\"map2\"]},\"segment.link.meta.tags\":{\"$all\":[\"tag1\"]}},\"limit\":10,\"skip\":15}" {
		fmt.Println("Segment query failed")
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

type FindSegmentsMockStub struct {
	shim.MockStub
}

func (p *FindSegmentsMockStub) GetQueryResult(queryString string) (shim.StateQueryIteratorInterface, error) {
	segment := cstesting.RandomSegment()
	segmentDoc := &SegmentDoc{
		ObjectType: ObjectTypeSegment,
		ID:         segment.GetLinkHashString(),
		Segment:    *segment,
	}
	iterator := segmentIterator{
		[]*SegmentDoc{segmentDoc},
	}
	return &iterator, nil
}

type segmentIterator struct {
	SegmentDocs []*SegmentDoc
}

func (s *segmentIterator) HasNext() bool {
	if len(s.SegmentDocs) > 0 {
		return true
	}
	return false
}
func (s *segmentIterator) Next() (*queryresult.KV, error) {
	if len(s.SegmentDocs) == 0 {
		return nil, errors.New("Empty")
	}
	segmentDoc := s.SegmentDocs[0]
	segmentDocBytes, err := json.Marshal(segmentDoc)
	if err != nil {
		return nil, err
	}

	s.SegmentDocs = s.SegmentDocs[1:]

	return &queryresult.KV{
		Key:       segmentDoc.ID,
		Value:     segmentDocBytes,
		Namespace: "mychannel",
	}, nil
}

func (s *segmentIterator) Close() error {
	return nil
}
