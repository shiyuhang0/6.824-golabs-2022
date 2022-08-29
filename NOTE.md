# 课程介绍
分布式的目的
- 并行度
- 容错
- 物理原因：物理上是分开的
- 安全性

分布式的挑战
- 并发
- partial failure（容错能力）：上千台电脑意味着小概率的错误变成了常见问题，故障总会出现
  - 可用性：某些故障时仍提供完好服务，太多故障仍不可用
  - recoverability：可恢复性，太多故障时可以采取措施恢复过来
  - 实现手段
    - 非易失性存储-写入慢
    - replication 复制
- performance - 可拓展性
- 一致性。多个数据副本的一致性（强弱）：更强的一致性意味着更多的通信，更大的 cost

基础架构-需要进行抽象
- 存储
- 计算
- 通信

lab
1. mapreduce
2. raft
3. k/v service
4. shard k/v service (分片)

# MapReduce

背景：大数据 TB 级别计算，并提供一个隐藏细节的框架，供业务开发人员使用

瓶颈：文件传输带来的带宽限制，如执行 map 时 master 要去分发数据，或者 worker 要去文件系统请求数据

优化
- 本地优化-> mapreduce 和 分布式文件系统一起部署，当需要对某个 input 执行 map 时，master 会去对应机器拉起 worker 直接本地执行 map（不过现在网络很快了，没这个优化也可以）

shuffle ：将各个 map 中相同的 key 汇聚到 reduce 中，这一步网络通信则不可避免


# Go 多线程 RPC
go的优点：
- 方便进行线程同步/锁机制
- 方便的 RPC 包
- 类型安全，内存安全
- 垃圾回收

goroutine 其实就是线程（程序计数器+一组寄存器+栈）main 其实就是 goroutine

为什么分布式关心多线程
- 可以实现并发 IO
- 多核并行
- 便利：如周期性检查

事件驱动编程(异步编程)也可以实现并发 IO：一条线程，一个循环（如 epoll） 
- 循环里会等待，获取到数据包后，判断它来自哪一个客户端。
- 优点是开销小高效（线程创建销毁调度的开销）
- 多核 CPU 可开多个线程，每个线程使用 异步IO

进程 与 线程
- 运行 go 时会创建一个 unix 进程并开辟地址空间，创建 gorotine 时就相当于创建了线程。
- 进程间地址独立，不可访问。线程见地址内存可以交互- 通过channle mutex 等

多线程的挑战
- Race：共享内存时，存在并发问题- 通常用锁解决，或者就不共享内存了
- coordination：(同步) 线程间协作 - channel sync.cond sync.waitgroup (java condition)
- Deadlock

网络爬虫 Code
```
// DFS

// lock

// 同步实现

```

# GFS

### 导读

大型分布式存储需要考虑什么？（why hard）

- performance -> 那么就要考虑如何进行数据拆分，即 sharding
- 分片带来 fault -> 解决 fault 即需要 tolerance
- tolerance 的实现 -> replica
- repl 带来的问题 -> in consistency
- 为了保证一致性带来的问题 -> low performance

糟糕的 replica 设计：

![img.png](imag/bad_repl.png)

一旦 2 个副本响应两个 client 的顺序不同，或者某个请求 down 了。都会导致 replica 数据不一致

GFS 的目标
- big & fast
- global: 可复用的存储系统（一套文件系统，而不是多套）
- sharding : 分片大数据，以得到高吞吐
- 自动故障恢复
- google 自己的场景：大文件连续访问（非随机访问

论文提出的一些关键点
- 存储系统可以具备若弱一致性
- 单一 master

### GFS 架构

Master + chunk 服务器 （分离了存储和元信息管理）
- master 负责文件命名+查询 chunk 位置
- chunk 服务器负责数据存储
- 每个文件被拆分成 64M 的多个 chunk 存储在多个 chunk 服务器中

Master 数据
- file name - chunk handles(标识符) 的数组 （映射关系） nv(非易失性，需存储到磁盘)
- chunk handle - 以下信息
  - chunk server 的列表（多副本） v
  - version    nv
  - primary    v
  - lease 过期时间  v

Master 如何故障恢复：log checkpoint - 持久化到 disk

Read 步骤
1. Client 发送 fileName + offset 请求 master
2. master 返回 handle 与 chunk server 列表
3. Client 查看是否有缓存，没有选择一个 chunk server 请求 chunk （可以是任何 chunk server ，不一定是 primary）
4. chunk server 返回 chunk 对应数据
5. （一般有一层 library-获取到 chunk 后，library 负责获取 chunk 中的所需数据返回给 client）

Write 步骤

0. client 发送写请求，master 基于文件名找到 chunk

1.没有 primary 的场景 - 以下在 master 上进行
  - master 找到最新的 chunk 副本：根据 version,master 会忽略比 version 更小的 repl。
  - 从中选择一个 chunk 为 primary ，其他为 secondary
  - 增加版本号
  - 告诉 chunk 副本 谁是 primary 与新的版本号（还会有 lease，如60s过期时间）
  - master 将版本号写入磁盘持久化

（版本号是为了选择最新的副本，过期时间是为了防、
双主- primary 挂了然后重启导致双主，或是网络分区导致了双主）


2. 然后基于 1PC 进行写入（GFS没有回滚）
  - primary 找到 offset
  - primary  secondary 都在该 offset 写入
  - secondary 成功返回 yes
  - 所有都成功了 primary 给客户端返回 yes，否则 error
  - 注意：如果部分成功，成功的那部分在 GFS 中不会进行回滚。依赖于 client 重试达到一致

