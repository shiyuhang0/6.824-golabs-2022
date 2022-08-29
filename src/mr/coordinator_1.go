package mr

//type Coordinator struct {
//	RunningMapTaskMap    map[int]*MapTask
//	RunningReduceTaskMap map[int]*ReduceTask
//	ActiveMapTasks       []*MapTask
//	ActiveReduceTasks    []*ReduceTask
//	ReduceN              int
//	mutex                sync.Mutex
//}
//
//type MapTask struct {
//	TaskId   int
//	FileName string
//	Status   int32 //0:NOTRUN 1:RUNNING 2:FINISHED
//}
//
//type ReduceTask struct {
//	TaskId   int
//	FileName []string
//	Status   int32
//}
//
//type TaskStatus int32
//
//const (
//	NOTRUN   TaskStatus = 0
//	RUNNNING TaskStatus = 1
//	FINISHED TaskStatus = 2
//)
//
//func (c *Coordinator) HandleWorker(args *CoordinatorArgs, reply *CoordinatorReply) error {
//	c.mutex.Lock()
//	if len(c.ActiveMapTasks) > 0 {
//		mapTask := c.ActiveMapTasks[0]
//		mapTask.Status = 1
//		c.ActiveMapTasks = c.ActiveMapTasks[1:]
//		c.RunningMapTaskMap[mapTask.TaskId] = mapTask
//
//		reply.TaskId = mapTask.TaskId
//		reply.FileName = []string{mapTask.FileName}
//		reply.TaskType = "MAP"
//		reply.ReduceN = c.ReduceN
//
//		go func(mapTask *MapTask) {
//			time.Sleep(10 * time.Second)
//			c.mutex.Lock()
//			ok := atomic.CompareAndSwapInt32(&mapTask.Status, 1, 0)
//			if ok {
//				c.ActiveMapTasks = append(c.ActiveMapTasks, mapTask)
//				delete(c.RunningMapTaskMap, mapTask.TaskId)
//			}
//			c.mutex.Unlock()
//		}(mapTask)
//	} else if len(c.RunningMapTaskMap) == 0 && len(c.ActiveReduceTasks) > 0 {
//		reduceTask := c.ActiveReduceTasks[0]
//		reduceTask.Status = 1
//		c.ActiveReduceTasks = c.ActiveReduceTasks[1:]
//		c.RunningReduceTaskMap[reduceTask.TaskId] = reduceTask
//
//		reply.TaskId = reduceTask.TaskId
//		reply.FileName = reduceTask.FileName
//		reply.TaskType = "REDUCE"
//		reply.ReduceN = c.ReduceN
//
//		go func(reduceTask *ReduceTask) {
//			time.Sleep(10 * time.Second)
//			c.mutex.Lock()
//			ok := atomic.CompareAndSwapInt32(&reduceTask.Status, 1, 0)
//			if ok {
//				c.ActiveReduceTasks = append(c.ActiveReduceTasks, reduceTask)
//				delete(c.RunningReduceTaskMap, reduceTask.TaskId)
//			}
//			c.mutex.Unlock()
//		}(reduceTask)
//	}
//	c.mutex.Unlock()
//	return nil
//}
//
//func (c *Coordinator) HandleFinish(args *CoordinatorFinishArgs, reply *CoordinatorFinishReply) error {
//	c.mutex.Lock()
//	if args.TaskType == "MAP" {
//
//		mapTask, ok := c.RunningMapTaskMap[args.Id]
//		if ok {
//			success := atomic.CompareAndSwapInt32(&mapTask.Status, 1, 2)
//			if success {
//				delete(c.RunningMapTaskMap, args.Id)
//			}
//		}
//	} else if args.TaskType == "REDUCE" {
//		reduceTask, ok := c.RunningReduceTaskMap[args.Id]
//		if ok {
//			success := atomic.CompareAndSwapInt32(&reduceTask.Status, 1, 2)
//			if success {
//				delete(c.RunningReduceTaskMap, args.Id)
//			}
//		}
//	}
//	c.mutex.Unlock()
//	return nil
//}
//
//// an example RPC handler.
////
//// the RPC argument and reply types are defined in rpc.go.
//func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
//	reply.Y = args.X + 1
//	return nil
//}
//
//// start a thread that listens for RPCs from worker.go
//func (c *Coordinator) server() {
//	rpc.Register(c)
//	rpc.HandleHTTP()
//	//l, e := net.Listen("tcp", ":1234")
//	sockname := coordinatorSock()
//	os.Remove(sockname)
//	l, e := net.Listen("unix", sockname)
//	if e != nil {
//		log.Fatal("listen error:", e)
//	}
//	go http.Serve(l, nil)
//}
//
//// main/mrcoordinator.go calls Done() periodically to find out
//// if the entire job has finished.
//func (c *Coordinator) Done() bool {
//	ret := false
//
//	c.mutex.Lock()
//	if len(c.RunningReduceTaskMap) == 0 && len(c.ActiveReduceTasks) == 0 {
//		ret = true
//	}
//	c.mutex.Unlock()
//
//	return ret
//}
//
//// create a Coordinator.
//// main/mrcoordinator.go calls this function.
//// nReduce is the number of reduce tasks to use.
//func MakeCoordinator(files []string, nReduce int) *Coordinator {
//
//	MapTaskMap := make(map[int]*MapTask)
//	ReduceTaskMap := make(map[int]*ReduceTask)
//	var ActiveMapTasks []*MapTask
//	var ActiveReduceTasks []*ReduceTask
//
//	for index, f := range files {
//		mapTask := &MapTask{
//			TaskId:   index + 1,
//			FileName: f,
//			Status:   0,
//		}
//		ActiveMapTasks = append(ActiveMapTasks, mapTask)
//	}
//
//	for i := 0; i < nReduce; i++ {
//		var fileName []string
//		for j := 0; j < len(files); j++ {
//			fileName = append(fileName, fmt.Sprintf("mr-%d-%d", j+1, i+1))
//		}
//		reduceTask := &ReduceTask{
//			TaskId:   i + 1,
//			FileName: fileName,
//			Status:   0,
//		}
//		ActiveReduceTasks = append(ActiveReduceTasks, reduceTask)
//	}
//
//	c := Coordinator{
//		RunningMapTaskMap:    MapTaskMap,
//		RunningReduceTaskMap: ReduceTaskMap,
//		ActiveMapTasks:       ActiveMapTasks,
//		ActiveReduceTasks:    ActiveReduceTasks,
//		ReduceN:              nReduce,
//	}
//
//	c.server()
//	return &c
//}
