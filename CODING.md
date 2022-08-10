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

## Logging

The log messages are divided into four basic categories: error, warning, info, debug, with a semantic meaning associated with each. Then there is a notion of V-level attached to the severity of debugging. V-levels provide a simple way of changing the verbosity of debug messages. V-levels provide a way for a given package to distinguish the relative importance or verbosity of a given log message. Then, if a particular logger or package logs too many messages, the package can simply change the V levels for that logger.

The key-value pairs (after a log message) should add more context to log messages. Keys are quite flexible and can contain more or less any string value. For consistency with existing code, there are a few conventions that should be followed:

- make your keys human-readable and be specific as much as possible (choose "peer_address" instead of "address" or "peer"; "batch_id" instead of "id" or "batch", etc.)
- be consistent throughout the codebase
- keys should naturally correspond to parts of the message string
- use lower case for simple keys and lower_snake_case for more complex keys

Although key names are mostly unrestricted, it is generally a good idea to stick to printable ascii characters, or at least to match the general character set of your log lines.

Two types of application users are identified: regular users and developers. Ordinary users should not be presented with confusing technical implementation details in log messages, but only with meaningful information regarding the functionality of the application in the form of meaningful statements. Developers are the users who know the implementation details and the technical details will help them in debugging the problematic state of the application.

That is, the same problem event can have two log lines, but with different severity. This is the case for a combination of Error/Debug or Warning/Debug, where Error or Warning is for the normal user and Debug is for the developer to help them investigate the problem. Log Info and Debug V-levels are informative about expected state changes, but Info, with operational information, is for ordinary users and Debug V-levels, with technical details, for developers.

Error and warning log messages should provide enough information to identify the problem in the code base without the need to record the file name and line number alongside them, although this information can be added using the logging library.

### Error

Error log messages are meant for regular users to be informed about the event that is happening which is not expected or may require intervention from the user for application to function properly. Log messages should be written as a statement, e.g. *"unable to connect to peer", "peer_address", r12345* and should not reveal technical details about internal implementation, such are package name, variable name or internal variable state. It should contain information only relevant to the user operating the application, concepts that can be operated on using exposed application interfaces.

### Warning

Warning log messages are also meant for regular users, as Error log messages, but are just informative about the state of the application that it can handle itself, by retry mechanism, for example. They should have the same form as the Error log messages.

### Info

Info log messages are informing users about the changes that are affecting the application state in the expected manner, or presenting the user the information that can be used to operate the application, such as *"node has successfully started" "listening_address", 12345*.

### Debug

Debug log messages are meant for developers to identify the problem that has happened. They should contain as much as possible internal technical information and applications state to be able to reproduce and debug the issue.

### Debug V-levels

Debug V-levels log messages are meant to inform developers about expected successful operations that are not relevant for regular users, but may be useful for understanding states in which application is going through. The form should be the same as Debug log messages.


### Commit Messages

For commit messages, follow [this guideline](https://www.conventionalcommits.org/en/v1.0.0/). Use reasonable length for the subject and body, ideally no longer than 72 characters. Use the imperative mood for subject lines. A few examples:

- Clean your room
- Close the door
- Take out the trash
