package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	for true {
		time.Sleep(time.Second)
		args := CoordinatorArgs{}
		reply := CoordinatorReply{}
		// call for a task
		ok := call("Coordinator.HandleWorker", &args, &reply)
		if !ok {
			break
		}
		if reply.TaskType == "MAP" {
			err := doMap(os.Args[1], reply.FileName, reply.TaskId, reply.ReduceN, mapf)
			if err == nil {
				finishArgs := CoordinatorFinishArgs{
					Id:       reply.TaskId,
					TaskType: reply.TaskType,
				}
				finishReply := CoordinatorFinishReply{}
				call("Coordinator.HandleFinish", &finishArgs, &finishReply)
			}
		} else if reply.TaskType == "REDUCE" {
			err := doReduce(os.Args[1], reply.FileName, reply.TaskId, reducef)
			if err == nil {
				finishArgs := CoordinatorFinishArgs{
					Id:       reply.TaskId,
					TaskType: reply.TaskType,
				}
				finishReply := CoordinatorFinishReply{}
				call("Coordinator.HandleFinish", &finishArgs, &finishReply)
			}
		} else {
			// do nothing here
		}
	}
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

func doMap(plugin string, fileNames []string, taskId, reduceN int, mapf func(string, string) []KeyValue) error {

	intermediate := []KeyValue{}
	for _, filename := range fileNames {
		file, err := os.Open(filename)
		if err != nil {
			log.Fatalf("cannot open %v", filename)
		}
		content, err := ioutil.ReadAll(file)
		if err != nil {
			log.Fatalf("cannot read %v", filename)
		}
		file.Close()
		kva := mapf(filename, string(content))
		intermediate = append(intermediate, kva...)
	}

	// split data into nReduce
	bulks := make(map[int][]KeyValue)
	for _, value := range intermediate {
		reduceNumber := ihash(value.Key) % reduceN
		bulk, ok := bulks[reduceNumber]
		if ok {
			bulks[reduceNumber] = append(bulk, value)
		} else {
			bulks[reduceNumber] = []KeyValue{value}
		}
	}

	for _, value := range bulks {
		sort.Sort(ByKey(value))
	}

	// write nReduce file
	for r := 0; r < reduceN; r++ {
		fileName := fmt.Sprintf("mr-%d-%d", taskId, r+1)
		ofile, _ := os.Create(fileName)
		bulk, ok := bulks[r]
		if ok {
			enc := json.NewEncoder(ofile)
			for _, kv := range bulk {
				err := enc.Encode(&kv)
				if err != nil {
					return err
				}
			}
		}
		ofile.Close()
	}
	return nil
}

func doReduce(plugin string, fileNames []string, taskId int, reducef func(string, []string) string) error {

	fileName := fmt.Sprintf("mr-out-%d", taskId)
	ofile, _ := os.Create(fileName)

	intermediate := []KeyValue{}
	for _, fileName := range fileNames {
		file, _ := os.Open(fileName)

		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			intermediate = append(intermediate, kv)
		}
	}

	// sort is necessary
	sort.Sort(ByKey(intermediate))
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)

		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)

		i = j
	}

	ofile.Close()

	return nil
}
