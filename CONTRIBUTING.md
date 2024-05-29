# Bee contributing guide

Thank you for investing your time in contributing to our project!

Before you start contributing please go through this file and also our [coding guidelines](./CODING.md) and [style guide](./CODINGSTYLE.md).

## Getting started

To get an overview of the project go through the [README](./README.md) file. Bee client is the Golang implementation of the Swarm decentralized storage layer as described in [The Book of Swarm](https://www.ethswarm.org/The-Book-of-Swarm.pdf). It is important to understand the concepts in this book before you work with the codebase.

For any questions you can reach out to us in [discord](https://discord.com/invite/GU22h2utj6).

## Issues

We use Github Issues for planning at present. The first interaction with the codebase should be creating an issue on Github. There are different issue templates already defined in the repository. Please use the appropriate template and fill the required information.

Lets go through some of them,

- Bugs
- Feature Requests
- Documentation Issue
- Task

The first three options are pretty self-explanatory. For example, you should be able to find some already in our [github issues](https://github.com/ethersphere/bee/issues) page.

As a team we follow the democratic approach to deciding the validity of issues. By creating issues, you are expected to get consensus within the team before you work on any PRs. The Bee client is largely stable codebase and has a whole ecosystem of projects that depend on it. We prioritize consistency and stability around our codebase.

### Task

Task template is a generic template used for working on changes in the codebase. Lets go through the different types of tasks that you could work on:

- Good first issues

These are work items that are good if you're contributing to the codebase for the first time, they're usually low priority for the Bee team, but they're definitely good to have. You can find the `good first issue` label in our issues. We will keep adding these from time to time. So if you don't know what to work on, it is highly recommended picking something with this label to start.

- Performance optimizations

  The best way to propose any optimizations would be to provide the relevant data to describe the problem and then also the same data after the optimizations are done. Keep in mind, Bee nodes work in a distributed system, so changes that would seem good locally may not hold in some cases. The Bee client can show you metrics as well as pprof information. This can be used to demonstrate the optimizations.

  - Concurrency related optimizations

    The Bee codebase uses concurrency in most of the packages. Concurrency is hard and before proposing any changes here it is expected that you can show clearly using pprof profiles and long-term testing data that your changes would improve the current situation.

  - Mutex related optimizations

    Needless to say, any proposed changes with mutexes or synchronization in general needs thorough data to back up. There could also be cases where some patterns are preferred by developers over other. The way to go about getting such changes through would be to get the team members on board with the proposal first before attempting any PRs.

- Code refactorings

  In the case where you manage to spend enough time in the codebase to figure out that some stuff can be refactored for idiomacy. This is not strictly discouraged. However, such changes are usually very subjective. So again the right way to go about these things is to clear them up in an issue before attempting any changes. As we mentioned before, Bee is a largely stable codebase and we prefer stability over most other things. There are some packages in the codebase on which a lot of other packages depend on. Also there is some historical context for some of the decisions. So it is important to involve the team beforehand in understanding that. Some of these packages also have equivalent javascript implementations. So they also need to be consistent across codebases. It would be difficult to point out all the packages here, but some examples would be:

  - `swarm` package which includes the core definitions of types that we use all around the codebase.

  - Core DISC abstractions

    Protocols like `PushSync`, `PullSync`, `hive`, `chainsync`, `retrieval`.

  - Core network abstractions

    `p2p`, `topology`, `accouting`.

  - Core storage abstractions

    `storage`, `localstore`, `postage`.


### Lifecycle of tasks

Once you have achieved the said consensus on the tasks you can start working on them. The basic Git flow is expected:

- Make changes locally.
- We use conventional commit pattern for creating the Git commits.
- Create a PR. There is already a template for this. Fill all the relevant information. You are encouraged to provide as much data as you can here. We really appreciate thoroughness!
- Get review comments.
- Resolve all the comments and make sure CI is green.

Congratulations :tada::tada: The Bee team thanks you :sparkles:.

## Code of conduct

- Please be kind and courteous in code-review comments. There’s no need to be mean or rude.
- Respect that people have differences of opinion and that every design or implementation choice carries a trade-off and numerous costs.
- Please keep unstructured critique to a minimum. If you have solid ideas you want to experiment with, Follow the step described in this guide.
- Keep in mind that the Bee team is responsible for maintaining the codebase and therefore has the right to make final decisions.
- Likewise any [brilliant jerks](https://www.brendangregg.com/blog/2017-11-13/brilliant-jerks.html), spamming, trolling, flaming, baiting or other attention-stealing behavior is not welcome.
