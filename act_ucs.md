# ACT user stories

This file contains the SWARM ACT user stories.

ZenHub Link: [SpDevTeam](https://app.zenhub.com/workspaces/spdevteam-6544d91246b817002d853e69/board)

### Context & definitions
- Content is created as a 'channel', i.e. as a feed with a specific topic.
- The content is uploaded to the feed, and new version of the content could be uploaded any time.
- The content can be accessed by the publisher and any viewers if it does not have ACT defined.
- The content can be accessed by the publisher and any viewers if it has ACT defined and the viewer is in the grantees list.
- A publisher is a user who is uploading new content.
- A viewer is a user who is accessing content.
- When a viewer is losing access it means that in the newly generated ACT the viewer will no longer be present in the grantee list.
- The publisher should be able to modify the ACT, using the same private key, but on multiple nodes in parallel.
- The publisher should be able to modify the grantee list, using the same private key, but on multiple nodes in parallel (this would potentially require the grantee list to be encrypted as well).

### Use cases

- [ ] **1**
- I'm a publisher
- I upload some new content
- I create an ACT for the content, but without additional grantees
- I can access the content
___

- [ ] **1/a**
- I'm a publisher
- I have an existing ACT for some content
- I can read and edit the ACT
___

- [ ] **2**
- I'm a publisher
- I have an existing ACT for some content, but without additional grantees
- I grant access to the content for some new viewers
- Those viewers can access the content
___

- [ ] **2/a**
- I'm a publisher
- I have an existing ACT for some content with some existing grantees
- I grant access to the content to additional viewers
- The existing viewers can access the content still
- The additional viewers can access the content
___

- [ ] **2/b**
- I'm a publisher
- I remove viewers from the grantee list of the ACT content
- The content is unchanged
- The removed viewers can access the content still
- The existing viewers can access the content still
___

- [ ] **2/c**
- I'm a publisher
- I remove viewers from the grantee list of the ACT content
- The content is updated
- The removed viewers can't access the latest version of content anymore
- The removed viewers can access an older version of the content, the version up until the moment they were removed
- The existing viewers can access the content still, including the latest version
___

- [ ] **3**
- I'm a viewer
- I requested access to the content
- If I was granted access, I can access the content
- If I was not granted access, I can't access the content still
___

- [ ] **4**
- I'm a viewer
- I had access to the content until now
- My access to the content got revoked
- The content is unchanged
- I can access the content still
___

- [ ] **4/a**
- I'm a viewer
- I had access to the content until now
- My access to the content got revoked
- The content is updated
- I can't access versions of the content that were uploaded after I lost access
___
