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

Log messages are categorized in five different severities: error, warning, info, debug and trace, with semantic meaning associated with each of them.

Two types of application users are identified: regular users and developers. Regular users should not be presented with confusing technical implementation details in log messages, but only with meaningful information related to application operability in a form of meaningful statements. Developers are users that are aware of implementation details and they will benefit from technical details to help them debug the problematic application state.

This means that the same problematic event may have two log lines but with different severities. This is the case with Error/Debug or Warning/Debug combo where Error or Warning is meant for the regular user and the Debug for developers to help them investigate the issue. Info and Trace log levels are informative about the expected state changes, but Info, with operable information, for regular users, and Trace, with technical details, for developers.

Error and warning log messages should provide enough information to identify a problem in the codebase without the need to log the filename and line number alongside, even if that information is possible to be added by the logging library.

### Error

Error log messages are meant for regular users to be informed about the event that is happening which is not expected or may require intervention from the user for application to function properly. Log messages should be written as a statement, e.g. *unable to connect to peer 12345* and should not reveal technical details about internal implementation, such are package name, variable name or internal variable state. It should contain information only relevant to the user operating the application, concepts that can be operated on using exposed application interfaces.

### Warning

Warning log messages are also meant for regular users, as Error log messages, but are just informative about the state of the application that it can handle itself, by retry mechanism, for example. They should have the same form as the Error log messages.

### Info

Info log messages are informing users about the changes that are affecting the application state in the expected manner, or presenting the user the information that can be used to operate the application, such as *node address: 12345*.

### Debug

Debug log messages are meant for developers to identify the problem that has happened. They should contain as much as possible internal technical information and applications state to be able to reproduce and debug the issue. Format of such log messages should be declarative with colon `:` as the separator between code concepts as nested call stacks, components or abstractions, e.g. *handshake: parse peer /p2p/Qm2313123 address 12345: encoding/hex: odd length hex string*, just like Go error values are chained with annotations.

### Trace

Trace log messages are meant to inform developers about expected successful operations that are not relevant for regular users, but may be useful for understanding states in which application is going through. The form should be the same as Debug log messages.
