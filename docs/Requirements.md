##  Functional Requirements
1. Persistentent storage of the object in distributed environment with worker nodes with replication factor of 2.
2. Persistent storage of objects metadata.
3. CRUD operation service on metadata.
4. Provide stored objects located in a distributed environment to concerned users.
5. Must be eventually available in case of worker node failure.
6. Maintain data integerity in case of corruption of data in one of worker node.

## Non Functional Requirements
1. Equal workload for all worker nodes (through good distribution policy).
2. modular and service level testing.
3. Client graphical ui on web browser. 
4. automated redistribution of workload in case a new worker node joins.
5. Freeing up the disk space not needed and reduncdant after use.

## Future scopes
1. removing single point of failure with multiple controllers.
2. Explore the possibility of using job queues for communication between controller and workers. (Need to think about how acknowledgement of job completion would work to notify the caller and most importantly we cannot provide objects in messages of job queues we may need to have a different http endpoint for that. and how beneficial would that be compared to some asynchronous behaviour when using http servers which do handle multiple request at same time using asynchronous event loops and multithreading) A single-threaded event loop (like Node.js) accepts requests and delegates blocking tasks (like disk or network I/O) to the operating system, switching between tasks rapidly.