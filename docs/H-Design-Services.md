## Controller - Requirements
- Client Facing api [[ClientApi.png]]
- persistent storage of metadata. (how to recover on restart). Persistent storage should contain object metadata excluding the chunking information cause that should be consistent with the available worker nodes and should be fetched from worker nodes.
- CRUD operations on metadata.
- fragementation of object into chunks and defragmentation of chunks back to object while persisting data.
- Distribution policy and storing dynamic information of chunks and thier locations.
- worker facing api [[internalAPI.png]]
	- detection of worker failure
	- worker start information exchange
- Replication of lost chunks on worker failure maintaining replication factor to 2. 
- Ensuring non corruption of data through checksum
	- repair copy from replica

## Worker-Requirements
- Controller facing api
- Create, delete, read operations service on chunks
- Recovery mechanism restart policy
