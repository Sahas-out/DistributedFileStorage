
## Api endpoints 
- PUT : /internals/chunks/{chunk_id} : request body : chunk_controller
- GET : /internals/chunks/{chunk_id} : response body : chunk_controller
- DELETE: /internals/chunks/{chunks_id}
- GET : /internals/chunks/metadata : response body : chunk:metadata_controller

## DTO
## CHUNK_Controller
- object_id :
- sequence_no :
- data :

## CHUNKMetadata_Controller
- chunk_id (PK)	Unique chunk identifier
- object_id	Parent object identifier
- sequence_no	Chunk's position in object
## CHUNKMetadata
- chunk_id (PK)	Unique chunk identifier
- object_id	Parent object identifier
- sequence_no	Chunk's position in object
- file_path	Location of chunk file
- size	Chunk size in bytes
- checksum	For integrity verification
- created_at	Creation timestamp


## Interfaces
### storage
- Put puts the object into the storage (register metadata entry and store in file)
- get gets the object from storage(retrieve some metadata and data in file)
- delete delete the object from storage (delete metadata entry and file)
- list  retrieves all the metadata information