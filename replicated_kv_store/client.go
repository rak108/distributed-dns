package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/krithikvaidya/distributed-dns/replicated_kv_store/protos"
)

//WriteCommand allows clients to submit a new command to the leader
func (node *RaftNode) WriteCommand(operation []string) bool {

	// True only if leader
	if node.state == Leader {
		//append to local log
		node.log = append(node.log, protos.LogEntry{Term: node.currentTerm, Operation: operation})

		var entries []*protos.LogEntry
		entries = append(entries, &node.log[len(node.log)-1])

		msg := &protos.AppendEntriesMessage{

			Term:         node.currentTerm,
			LeaderId:     node.replica_id,
			PrevLogIndex: int32(len(node.log) - 1),
			PrevLogTerm:  node.log[len(node.log)-1].Term,
			LeaderCommit: node.commitIndex,
			Entries:      entries,
		}

		successful_write := make(chan bool)
		node.LeaderSendAEs(operation[0], msg, int32(len(node.log)-1), successful_write)

		success := <-successful_write //Written to from AE when majority of nodes have replicated the write or failure occurs

		if success {
			node.commitIndex++
			log.Printf("\nWrite operation successfully completed and committed.\n")
		} else {
			log.Printf("\nWrite operation failed.\n")
		}

		return success
	}

	return false
}

// ReadCommand is different since read operations do not need to be added to log
func (node *RaftNode) ReadCommand(key int) (string, error) {

	write_success := make(chan bool)
	node.StaleReadCheck(write_success)
	status := <-write_success

	if (status == true) && (node.state == Leader) {

		// assuming that if an operation on the state machine succeeds on one of the replicas,
		// it will succeed on all. and vice versa.
		url := fmt.Sprintf("http://localhost%s/%d", node.kvstore_addr, key)

		resp, err := http.Get(url)

		if err == nil {

			defer resp.Body.Close()
			contents, err := ioutil.ReadAll(resp.Body)

			log.Printf("\nREAD successful.\n")

			return string(contents), err

		} else {
			return "error occured", err
		}
	}

	return "unable to perform read", errors.New("read_failed")
}

// StaleReadCheck sends dummy heartbeats to make sure that a new leader has not come
func (node *RaftNode) StaleReadCheck(write_success chan bool) {
	replica_id := 0

	var entries []*protos.LogEntry

	node.raft_node_mutex.RLock()

	prevLogIndex := node.nextIndex[replica_id] - 1
	prevLogTerm := int32(-1)

	if prevLogIndex >= 0 {
		prevLogTerm = node.log[prevLogIndex].Term
	}

	hbeat_msg := &protos.AppendEntriesMessage{

		Term:         node.currentTerm,
		LeaderId:     node.replica_id,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		LeaderCommit: node.commitIndex,
		Entries:      entries,
	}

	node.raft_node_mutex.RUnlock()

	node.LeaderSendAEs("HBEAT", hbeat_msg, int32(len(node.log)), write_success)
}
