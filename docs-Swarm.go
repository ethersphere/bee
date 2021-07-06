Swarm API (0.6.0)
Download OpenAPI specification:Download

A list of the currently provided Interfaces to interact with the swarm, implementing file operations and sending messages

Browse the documentation @ the Swarm Docs
Bytes
Upload data
AUTHORIZATIONS:
HEADER PARAMETERS
swarm-tag	
integer
Associate upload with an existing Tag UID

swarm-pin	
boolean
Represents the pinning state of the chunk

swarm-encrypt	
boolean
Represents the encrypting state of the file

swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

REQUEST BODY SCHEMA: application/octet-stream
string <binary>
Responses
201 Ok
400 Bad request
402 Payment Required
403 Forbidden
500 Internal Server Error
default Default response

POST
/bytes
Response samples
201400402403500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Get referenced data
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Swarm address reference to content

Responses
200 Retrieved content specified by reference
404 Not Found
default Default response

GET
/bytes/{reference}
Response samples
404
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Chunk
Get Chunk
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Swarm address of chunk

QUERY PARAMETERS
targets	
string
Global pinning targets prefix

Responses
200 Retrieved chunk content
202 chunk recovery initiated. retry after sometime.
400 Bad request
404 Not Found
500 Internal Server Error
default Default response

GET
/chunks/{reference}
Response samples
400404500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Upload Chunk
AUTHORIZATIONS:
HEADER PARAMETERS
swarm-tag	
integer
Associate upload with an existing Tag UID

swarm-pin	
boolean
Represents the pinning state of the chunk

swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

REQUEST BODY SCHEMA: application/octet-stream
string <binary>
Responses
201 Ok
400 Bad request
402 Payment Required
500 Internal Server Error
default Default response

POST
/chunks
Response samples
201400402500
Content type
application/json

Copy
Expand allCollapse all
{
"status": "string"
}
File
Upload file or a collection of files
In order to upload a collection, user can send a multipart request with all the files populated in the form data with appropriate headers.

User can also upload a tar file along with the swarm-collection header. This will upload the tar file after extracting the entire directory structure.

If the swarm-collection header is absent, all requests (including tar files) are considered as single file uploads.

A multipart request is treated as a collection regardless of whether the swarm-collection header is present. This means in order to serve single files uploaded as a multipart request, the swarm-index-document header should be used with the name of the file.

AUTHORIZATIONS:
QUERY PARAMETERS
name	
string
Filename when uploading single file

HEADER PARAMETERS
swarm-tag	
integer
Associate upload with an existing Tag UID

swarm-pin	
boolean
Represents the pinning state of the chunk

swarm-encrypt	
boolean
Represents the encrypting state of the file

Content-Type	
string
The specified content-type is preserved for download of the asset

swarm-collection	
boolean
Upload file/files as a collection

swarm-index-document	
string
Example: index.html
Default file to be referenced on path, if exists under that path

swarm-error-document	
string
Example: error.html
Configure custom error document to be returned when a specified path can not be found in collection

swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

REQUEST BODY SCHEMA: 
multipart/form-data
file	
Array of strings <binary>
Responses
201 Ok
400 Bad request
402 Payment Required
403 Forbidden
500 Internal Server Error
default Default response

POST
/bzz
Response samples
201400402403500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Collection
Upload file or a collection of files
In order to upload a collection, user can send a multipart request with all the files populated in the form data with appropriate headers.

User can also upload a tar file along with the swarm-collection header. This will upload the tar file after extracting the entire directory structure.

If the swarm-collection header is absent, all requests (including tar files) are considered as single file uploads.

A multipart request is treated as a collection regardless of whether the swarm-collection header is present. This means in order to serve single files uploaded as a multipart request, the swarm-index-document header should be used with the name of the file.

AUTHORIZATIONS:
QUERY PARAMETERS
name	
string
Filename when uploading single file

HEADER PARAMETERS
swarm-tag	
integer
Associate upload with an existing Tag UID

swarm-pin	
boolean
Represents the pinning state of the chunk

swarm-encrypt	
boolean
Represents the encrypting state of the file

Content-Type	
string
The specified content-type is preserved for download of the asset

swarm-collection	
boolean
Upload file/files as a collection

swarm-index-document	
string
Example: index.html
Default file to be referenced on path, if exists under that path

swarm-error-document	
string
Example: error.html
Configure custom error document to be returned when a specified path can not be found in collection

swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

REQUEST BODY SCHEMA: 
multipart/form-data
file	
Array of strings <binary>
Responses
201 Ok
400 Bad request
402 Payment Required
403 Forbidden
500 Internal Server Error
default Default response

POST
/bzz
Response samples
201400402403500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Get file or index document from a collection of files
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Swarm address of content

QUERY PARAMETERS
targets	
string
Global pinning targets prefix

Responses
200 Ok
400 Bad request
404 Not Found
500 Internal Server Error
default Default response

GET
/bzz/{reference}
Response samples
400404500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Get referenced file from a collection of files
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Swarm address of content

path
required
string
Path to the file in the collection.

QUERY PARAMETERS
targets	
string
Global pinning targets prefix

Responses
200 Ok
400 Bad request
404 Not Found
500 Internal Server Error
default Default response

GET
/bzz/{reference}/{path}
Response samples
400404500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Reupload file
Reupload a root hash to the network
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Root hash of content

Responses
200 Ok
500 Internal Server Error
default Default response

PATCH
/bzz/{reference}
Response samples
500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Reupload collection
Reupload a root hash to the network
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string or string
Root hash of content

Responses
200 Ok
500 Internal Server Error
default Default response

PATCH
/bzz/{reference}
Response samples
500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Tag
Get list of tags
AUTHORIZATIONS:
QUERY PARAMETERS
offset	
integer >= 0
Default: 0
The number of items to skip before starting to collect the result set.

limit	
integer [ 1 .. 1000 ]
Default: 100
The numbers of items to return.

Responses
200 List of tags
403 Forbidden
500 Internal Server Error
default Default response

GET
/tags
Response samples
200403500
Content type
application/json

Copy
Expand allCollapse all
{
"tags": [
{}
]
}
Create Tag
AUTHORIZATIONS:
REQUEST BODY SCHEMA: application/json
address	
string ^[A-Fa-f0-9]{64}$
Responses
201 New Tag Info
403 Forbidden
500 Internal Server Error
default Default response

POST
/tags
Request samples
Payload
Content type
application/json

Copy
Expand allCollapse all
{
"address": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Response samples
201403500
Content type
application/json

Copy
Expand allCollapse all
{
"uid": 0,
"startedAt": "2020-06-11T11:26:42.6969797+02:00",
"total": 0,
"processed": 0,
"synced": 0
}
Get Tag information using Uid
AUTHORIZATIONS:
PATH PARAMETERS
uid
required
integer
Uid

Responses
200 Tag info
400 Bad request
403 Forbidden
404 Not Found
500 Internal Server Error
default Default response

GET
/tags/{uid}
Response samples
200400403404500
Content type
application/json

Copy
Expand allCollapse all
{
"uid": 0,
"startedAt": "2020-06-11T11:26:42.6969797+02:00",
"total": 0,
"processed": 0,
"synced": 0
}
Delete Tag information using Uid
AUTHORIZATIONS:
PATH PARAMETERS
uid
required
integer
Uid

Responses
204 The resource was deleted successfully.
400 Bad request
403 Forbidden
404 Not Found
500 Internal Server Error
default Default response

DELETE
/tags/{uid}
Response samples
400403404500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Update Total Count and swarm hash for a tag of an input stream of unknown size using Uid
AUTHORIZATIONS:
PATH PARAMETERS
uid
required
integer
Uid

REQUEST BODY SCHEMA: application/json
Can contain swarm hash to use for the tag

address	
string ^[A-Fa-f0-9]{64}$
Responses
200 Ok
403 Forbidden
404 Not Found
500 Internal Server Error
default Default response

PATCH
/tags/{uid}
Request samples
Payload
Content type
application/json

Copy
Expand allCollapse all
{
"address": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Response samples
200403404500
Content type
application/json

Copy
Expand allCollapse all
{
"status": "string"
}
Root hash pinning
Pin the root hash with the given reference
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string
Swarm reference of the root hash

Responses
200 Pin already exists, so no operation
201 New pin with root reference was created
400 Bad request
403 Forbidden
404 Not Found
500 Internal Server Error
default Default response

POST
/pins/{reference}
Response samples
200201400403404500
Content type
application/json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Unpin the root hash with the given reference
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string
Swarm reference of the root hash

Responses
200 Unpinning root hash with reference
400 Bad request
403 Forbidden
500 Internal Server Error
default Default response

DELETE
/pins/{reference}
Response samples
200400403500
Content type
application/json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Get pinning status of the root hash with the given reference
AUTHORIZATIONS:
PATH PARAMETERS
reference
required
string or string
Swarm reference of the root hash

Responses
200 Reference of the pinned root hash
400 Bad request
403 Forbidden
404 Not Found
500 Internal Server Error
default Default response

GET
/pins/{reference}
Response samples
200400403404500
Content type
application/json

Copy
Expand allCollapse all
"36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f2d2810619d29b5dbefd5d74abce25d58b81b251baddb9c3871cf0d6967deaae2"
Get the list of pinned root hash references
AUTHORIZATIONS:
Responses
200 List of pinned root hash references
403 Forbidden
500 Internal Server Error
default Default response

GET
/pins
Response samples
200403500
Content type
application/json

Copy
Expand allCollapse all
{
"addresses": [
"36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
]
}
Postal Service for Swarm
Send to recipient or target with Postal Service for Swarm
AUTHORIZATIONS:
PATH PARAMETERS
topic
required
string
Topic name

targets
required
string
Target message address prefix. If multiple targets are specified, only one would be matched.

QUERY PARAMETERS
recipient	
string
Recipient publickey

HEADER PARAMETERS
swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

Responses
201 Subscribed to topic
400 Bad request
402 Payment Required
500 Internal Server Error
default Default response

POST
/pss/send/{topic}/{targets}
Response samples
400402500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Subscribe for messages on the given topic.
AUTHORIZATIONS:
PATH PARAMETERS
topic
required
string
Topic name

Responses
200 Returns a WebSocket with a subscription for incoming message data on the requested topic.
500 Internal Server Error
default Default response

GET
/pss/subscribe/{topic}
Response samples
500
Content type
application/problem+json

Copy
Expand allCollapse all
{
"message": "string",
"code": 0
}
Single owner chunk
Upload single owner chunk
AUTHORIZATIONS:
PATH PARAMETERS
owner
required
string ^[A-Fa-f0-9]{40}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906
Owner

id
required
string ^([A-Fa-f0-9]+)$
Example: cf880b8eeac5093fa27b0825906c600685
Id

QUERY PARAMETERS
sig
required
string ^([A-Fa-f0-9]+)$
Example: sig=cf880b8eeac5093fa27b0825906c600685
Signature

HEADER PARAMETERS
swarm-pin	
boolean
Represents the pinning state of the chunk

Responses
201 Created
400 Bad request
401 Unauthorized
402 Payment Required
500 Internal Server Error
default Default response

POST
/soc/{owner}/{id}
Response samples
201400401402500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Feed
Create an initial feed root manifest
AUTHORIZATIONS:
PATH PARAMETERS
owner
required
string ^[A-Fa-f0-9]{40}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906
Owner

topic
required
string ^([A-Fa-f0-9]+)$
Example: cf880b8eeac5093fa27b0825906c600685
Topic

QUERY PARAMETERS
type	
string ^(sequence|epoch)$
Feed indexing scheme (default: sequence)

HEADER PARAMETERS
swarm-pin	
boolean
Represents the pinning state of the chunk

swarm-postage-batch-id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
ID of Postage Batch that is used to upload data with

Responses
201 Created
400 Bad request
401 Unauthorized
402 Payment Required
500 Internal Server Error
default Default response

POST
/feeds/{owner}/{topic}
Response samples
201400401402500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Find feed update
AUTHORIZATIONS:
PATH PARAMETERS
owner
required
string ^[A-Fa-f0-9]{40}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906
Owner

topic
required
string ^([A-Fa-f0-9]+)$
Example: cf880b8eeac5093fa27b0825906c600685
Topic

QUERY PARAMETERS
at	
integer
Timestamp of the update (default: now)

type	
string ^(sequence|epoch)$
Feed indexing scheme (default: sequence)

Responses
200 Latest feed update
400 Bad request
401 Unauthorized
500 Internal Server Error
default Default response

GET
/feeds/{owner}/{topic}
Response samples
200400401500
Content type
application/json

Copy
Expand allCollapse all
{
"reference": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
Postage Stamps
Get all available stamps for this node
AUTHORIZATIONS:
Responses
200 Returns an array of all available postage batches.
default Default response

GET
/stamps
Response samples
200
Content type
application/json

Copy
Expand allCollapse all
{
"stamps": [
{}
]
}
Get an individual postage batch status
AUTHORIZATIONS:
PATH PARAMETERS
id
required
string ^[A-Fa-f0-9]{64}$
Example: 36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f
Swarm address of the stamp

Responses
200 Returns an individual postage batch state
400 Bad request
default Default response

GET
/stamps/{id}
Response samples
200400
Content type
application/json

Copy
Expand allCollapse all
{
"batchID": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f",
"utilization": 0,
"usable": true,
"label": "string",
"depth": 0,
"amount": "1000000000000000000",
"bucketDepth": 0,
"blockNumber": 0,
"immutableFlag": true
}
Buy a new postage batch. Be aware, this endpoint create an on-chain transactions and transfers BZZ from the node's Ethereum account and hence directly manipulates the wallet balance!
AUTHORIZATIONS:
PATH PARAMETERS
amount
required
integer
Amount of BZZ added that the postage batch will have.

depth
required
integer
Batch depth which specifies how many chunks can be signed with the batch. It is a logarithm. Must be higher than default bucket depth (16)

QUERY PARAMETERS
label	
string
An optional label for this batch

HEADER PARAMETERS
gas-price	
integer
Gas price for transaction

Responses
201 Returns the newly created postage batch ID
400 Bad request
500 Internal Server Error
default Default response

POST
/stamps/{amount}/{depth}
Response samples
201400500
Content type
application/json

Copy
Expand allCollapse all
{
"batchID": "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
