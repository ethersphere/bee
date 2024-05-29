# Coding guide

This project is written in Go programming language and follows a community accepted coding styles and practices defined in [Effective Go](https://golang.org/doc/effective_go.html) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments). This document defines additional guidelines specific to the Bee project.

Developers should keep their Go tooling up to date with the latest stable versions.

## Static code analysis

Linting is done by using https://github.com/golangci/golangci-lint with configuration defined in the root of the project and also `go vet`. Executing `make lint vet` should pass without any warnings or errors.

## Testing

It is preferred to use a separate test package to write tests. This allows explicit testing on exported behaviour of the package, not the internal implementation. In need to export only to testing package, `export_test.go` file should be used. Testing internal implementations is discouraged, but not forbidden. It is preferred to treat exported package behaviours as units that need to be tested.

Executing `make test` should pass without any warnings or errors.

## Packages

Go packages with the Bee project should have a single and well defined responsibility, clear exposed API and tests that cover expected behaviour. To ensure better modularity of the codebase, every package should be treated as a library that provides a specific functionality and that can be used as a module in other applications. Every package should have a well written godoc page which should be used as the entry point for understanding the package's responsibility. The same as using any other third-party package. If the package godoc is not clear and requires looking at the code to understand behaviour that it provides, documentation should be improved.

## Concurrency

Handling concurrency in Go is not a trivial task.

For every goroutine that is created it must be defined how and when it terminates.

Every channel must have an owning goroutine. That goroutine is the only one that can send to the channel, close it or transfer ownership to another goroutine. Wherever is possible a readonly or writeonly channels should be used.

## Error propagation, annotation and handling

Errors must be propagated by the caller function. Error should not be logged and passed at the same time. It is up to the next caller function to decide what should be done in case of an error.

If a function has multiple returns with, any returned error should be annotated either with `fmt.Errorf` using `%w` verb or with custom error type that has `Unwrap() error` method. This approach has the advantage of resulting in an error message that is equivalent to a stack trace, since errors are repeatedly wrapped while being passed up the call stack until their eventual logging.

Explicit `error` values that are constructed with `errors.New` may have a message prefixed with the package name and a colon character, if it would improve the message clarity.

## Logging Guidelines

This document describes the basic principles of logging which should be followed when working with the code base. Following these principles will not only make the logging output more coherent, but can significantly reduce time spent searching for bugs and make it easier for node operators to navigate the state of their actively running node.

### Log Levels

The log messages are divided into four basic levels:

- `Error` - Errors in the node. Although the node operation may continue, the error indicates that it should be addressed.
- `Warning` - Warnings should be checked in case the problem recurs to avoid potential damage.
- `Info` - Informational messages useful for node operators that do not indicate any fault or error.
- `Debug` - Information concerning program logic decisions, diagnostic information, internal state, etc. which is primarily useful for developers.

There is a notion of `V-level` attached to the `Debug` level. `V-levels` provide a simple way of changing the verbosity of debug messages. `V-levels` provide a way for a given package to distinguish the relative importance or verbosity of a given log message. Then, if a particular logger or package logs too many messages, the package can simply change the `V` level for that logger.

### Log Messages

The key-value pairs (after a log message) should add more context to log messages. Keys are quite flexible and can contain more or less any string value. For consistency with existing code, there are a few conventions that should be followed:

- Make your keys human-readable and be specific as much as possible (choose "peer_address" instead of "address" or "peer", "batch_id" instead of "id" or "batch", etc.)
- Be consistent throughout the code base.
- Keys should naturally correspond to parts of the message string.
- Use lower case for simple keys and lower_snake_case for more complex keys.

Although key names are mostly unrestricted, it is generally a good idea to stick to printable ASCII characters, or at least to match the general character set of your log lines.

`Error` and `Warning` log messages should provide enough information to identify the problem in the code base without the need to record the file name and line number alongside them, although this information can be added using the logging library.

If you are writing/changing logs, it is recommended to look at the output. Answer questions like: is this what you expected? Is anything missing? Are they understandable? Do you need more/less context?

### Users

Two types of application users are identified: node operators and developers. Node operators should not be presented with confusing technical implementation details in log messages, but only with meaningful information regarding the functionality of the application in the form of meaningful statements. Developers are the users who know the implementation details and the technical details will help them in debugging the problematic state of the application.

In general, the goal for developers is to create log messages that are easy to understand, analyze, and reference code so they can troubleshoot issues quickly. In the case of node operators, log messages should be concise and provide all the necessary information without unnecessary details.

That is, the same problem event can have two log lines, but with different severity. This is the case for a combination of `Error/Debug` or `Warning/Debug`, where `Error` or `Warning` is for the node operator and `Debug` is for the developer to help them investigate the problem. Log `Info` and `Debug V-levels` are informative about expected state changes, but `Info`, with operational information, is for node operators and `Debug V-levels`, with technical details, for developers.

> The `Error` level should be used with caution when it comes to node operators. It is recommended to log application code errors at the `Error` level if they require node operator intervention or if the node cannot continue its normal operation (unrecoverable errors). Otherwise, recoverable (application code) errors should be logged at the `Debug` level with an appropriate `V-level`. The `Debug` level should not be used to monitor the node, but metrics should be used for this purpose.

Log lines should not contain the following attributes (remove, mask, sanitize, hash, or encrypt the following):

- application source code
- session identification values
- access tokens
- passwords
- database connection strings
- encryption keys and other primary secrets and sensitive information

### Recommendations

Some useful logging recommendations for cleaner output:

- Error - Error log messages are meant for regular users to be informed about the event that is happening which is not expected or may require intervention from the user for application to function properly. Log messages should be written as a statement, e.g. *"unable to connect to peer", "peer_address", r12345* and should not reveal technical details about internal implementation, such are package name, variable name or internal variable state. It should contain information only relevant to the user operating the application, concepts that can be operated on using exposed application interfaces.
- Warning - Warning log messages are also meant for regular users, as Error log messages, but are just informative about the state of the application that it can handle itself, by retry mechanism, for example. They should have the same form as the Error log messages.
- Info - Info log messages are informing users about the changes that are affecting the application state in the expected manner, or presenting the user the information that can be used to operate the application, such as *"node has successfully started" "listening_address", 12345*.
- Debug - Debug log messages are meant for developers to identify the problem that has happened. They should contain as much as possible internal technical information and applications state to be able to reproduce and debug the issue.
- Debug V-levels - Debug V-levels log messages are meant to inform developers about expected successful operations that are not relevant for regular users, but may be useful for understanding states in which application is going through. The form should be the same as Debug log messages.

- Do not log a function entry unless it is significant or unless it is logged at the debug level.
- Avoid logging from many iterations of a loop. It is OK to log from iterations of small loops or to periodically log from large loops.
- Truncate or summarize large messages or files in some way that will be useful for debugging.
- Avoid errors that are not really errors and can confuse the node operator. This sometimes happens when an application error is part of a successful execution flow.
- Do not log the same or similar errors repeatedly. It can quickly clutter the log and hide the real cause. The frequency of error types is best addressed by monitoring, and logs need to capture details for only some of these errors.

### Logger API

In the current Bee code base, it is possible to change the granularity of logging for some services on the fly without the need to restart the node.
These services and their corresponding loggers can be found using the `/loggers` endpoint. Example of the output:

```json
{
  "tree": {
    "node": {
      "/": {
        "api": {
          "+": [
            "info|node/api[0][]>>824634933256"
          ]
        },
        "batchstore": {
          "+": [
            "info|node/batchstore[0][]>>824634933256"
          ]
        },
        "leveldb": {
          "+": [
            "info|node/leveldb[0][]>>824634933256"
          ]
        },
        "pseudosettle": {
          "+": [
            "info|node/pseudosettle[0][]>>824634933256"
          ]
        },
        "pss": {
          "+": [
            "info|node/pss[0][]>>824634933256"
          ]
        },
        "storer": {
          "+": [
            "info|node/storer[0][]>>824634933256"
          ]
        }
      },
      "+": [
        "info|node[0][]>>824634933256"
      ]
    }
  },
  "loggers": [
    {
      "logger": "node/api",
      "verbosity": "info",
      "subsystem": "node/api[0][]>>824634933256",
      "id": "bm9kZS9hcGlbMF1bXT4-ODI0NjM0OTMzMjU2"
    },
    {
      "logger": "node/storer",
      "verbosity": "info",
      "subsystem": "node/storer[0][]>>824634933256",
      "id": "bm9kZS9zdG9yZXJbMF1bXT4-ODI0NjM0OTMzMjU2"
    },
    {
      "logger": "node/pss",
      "verbosity": "info",
      "subsystem": "node/pss[0][]>>824634933256",
      "id": "bm9kZS9wc3NbMF1bXT4-ODI0NjM0OTMzMjU2"
    },
    {
      "logger": "node/pseudosettle",
      "verbosity": "info",
      "subsystem": "node/pseudosettle[0][]>>824634933256",
      "id": "bm9kZS9wc2V1ZG9zZXR0bGVbMF1bXT4-ODI0NjM0OTMzMjU2"
    },
    {
      "logger": "node",
      "verbosity": "info",
      "subsystem": "node[0][]>>824634933256",
      "id": "bm9kZVswXVtdPj44MjQ2MzQ5MzMyNTY="
    },
    {
      "logger": "node/leveldb",
      "verbosity": "info",
      "subsystem": "node/leveldb[0][]>>824634933256",
      "id": "bm9kZS9sZXZlbGRiWzBdW10-PjgyNDYzNDkzMzI1Ng=="
    },
    {
      "logger": "node/batchstore",
      "verbosity": "info",
      "subsystem": "node/batchstore[0][]>>824634933256",
      "id": "bm9kZS9iYXRjaHN0b3JlWzBdW10-PjgyNDYzNDkzMzI1Ng=="
    }
  ]
}
```

The recorders come in two versions. The first is the tree version and the second is the flattened version. The `subsystem` field is the unique identifier of the logger.
The `id` field is a version of the `subsystem` field encoded in base 64 for easier reference to a particular logger.
The node name of the version tree is composed of the subsystem with the log level prefix and delimited by the `|` character.
The number in the first square bracket indicates the logger's V-level.

The logger endpoint uses HTTP PUT requests to modify the verbosity of the logger(s).
The request must have the following parameters `/loggers/{subsystem}/{verbosity}`.
The `{subsytem}` parameter is the base64 version of the subsytem field or regular expression corresponding to multiple subsystems.
Since the loggers are arranged in tree structure, it is possible to turn on/off or change the logging level of the entire tree or just its branches with a single command.
The verbosity can be one of `none`, `error`, `warning`, `info`, `debug` or a number in the range `1` to `1<<<31 - 1` to enable the verbosity of a particular V-level, if available for a given logger.
A value of `all` will enable the highest verbosity of V-level.

Examples:

`curl -XPUT http://localhost:1633/loggers/bm9kZS8q/none` - will disable all loggers; `bm9kZS8q` is base64 encoded `node/*` regular expression.

`curl -XPUT http://localhost:1633/loggers/bm9kZS9hcGlbMV1bXT4-ODI0NjM0OTMzMjU2/error` - will set the verbosity of the logger with the subsystem `node/api[1][]>>824634933256` to `error`.

## Commit Messages

For commit messages, follow [this guideline](https://www.conventionalcommits.org/en/v1.0.0/). Use reasonable length for the subject and body, ideally no longer than 72 characters. Use the imperative mood for subject lines. A few examples:

- Clean your room
- Close the door
- Take out the trash
