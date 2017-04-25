# How to pass objects around in Go?

There are a number of implications of how verbose or readable passing around
objects in Go is. I decided to write up a simple example that would use two
handlers, define two services and try to run some simple process that would
use them all.

The Writer handler writes a set of primary numbers to a Redis object.
The Processor handler reads the set of numbers from Redis, and calculates
the Sum/Count of the numbers in the Database object.

## Factory

This example uses a factory-object that can be passed to any/all handlers
which you define. This means that you have the complete selection over all
services at the system via getter functions.

~~~
func (r *Writer) Run(services *service.Services) {
~~~

The `*service.Services` object contains things like `GetDatabase` and
`GetRedis` functions, which take parameters and return a specific object.
Full control of what kind of object may be returned is in the definition
of the Services object (return types) and implementation of the getter
functions (can return a singleton, or new instances each time).

Verbosity statistics:

~~~
1 import per handler, 1 in main (3 total)
1 argument to handler (2 total)
1 line to consume service (3 SLOC total)
12 SLOC for service declaration
0 external packages
------
20 lines to create/pass 3 objects, 0 external packages
3.5 lines gain per handler
~~~

The good:

1. Adding a new service doesn't require modifying existing handlers,
2. Lazy loading - a service can be created on first use

The bad:

1. No clear indication of where a given service is used,
2. Poor documentation of which services a handler uses,
3. Service is declared in an importable package and not main,
4. Poorly testable due to imported service declaration

## Interface

This example uses an interface to declare the getters and resulting objects
which are expected from the interface.

~~~
type ProcessorServices interface {
	GetDatabase(string) *service.Database
	GetRedis(string) *service.Redis
}
~~~

Any object that satisfied this interface can be passed to the handlers.

Verbosity statistics:

~~~
1 import per handler, 1 in main (3 total)
1 argument to handler (2 total)
1 line to consume service (3 SLOC total)
2 per handler + 1 line per service in handler for interface (2*2+1+2=7)
12 SLOC for service declaration
0 external packages
------
27 lines to create/pass 3 objects, 0 external packages
8 lines gain per handler
~~~

The good:

1. Adding a service is well documented within the handler `Services` inteface,
2. Clear indication of where given services are used,
3. Lazy loading - a service can be created on first use
4. Testable - can inject services that satisfy interface

The bad:

1. Service is still declared in an importable package and not main,
2. Verbosity increases per handler/service,
3. Handlers don't have good documentation visible from main

## Interface 2 (refactor)

The example is basically the same, except we are trying to solve the "bad"
characteristics of the previous implementation. I moved the `service.Services`
declaration into `main.serviceFactory` so that service implementation is
separate from declaration of service getters. Verbosity stays the same.

These things make the handlers and individual services highly reusable, from
either a FQDN import or a local package which you can copy around between
projects depending on use.

Projects/applications are not created daily, so this is just a convenience factor.
Of course, there's nothing stopping you from using it as a FQDN/vendored resource.

## Struct

This example tries to use a declarative approach to the services which a handler
may use. This way, we remove the getters and resort to use actual instances of
the structs which any handler requires.

~~~
type Processor struct {
	DB    *service.Database `inject`
	Redis *service.Redis    `inject`
}
~~~

As handlers already have access to the objects, the verbosity will go down because
we don't have getters anymore, and the handlers don't take their dependencies over
the parameters.

Verbosity statistics:

~~~
1 import per handler, 1 in main (3 total)
0 arguments to handler (0 total)
0 lines to consume service (0 SLOC total)
1 line per service in handler declaration (1+2=3)
6 SLOC for service declaration in handler (3/handler)
0 external packages
------
12 lines to create/pass 3 objects, 0 external packages
2.5 lines gain per handler, 2 lines/service population
~~~

The good:

1. Handlers have two-way documentation (clear usage from main pkg as well),
3. Services are declared in the main package,
4. Testable - can assign services based on handler struct

The bad:

1. No lazy loading - can't use lazy loading without resorting back to factories
2. Low verbosity but significantly increasing with number of handlers,
3. Time sink during changes in development

As each handlers services are populated by hand, the documentation value of the
main package is improved, because visual inspection of handlers becomes possible.
However, as you add or remove services to your handlers during development, you
have to jump between main and your handler to avoid populating the services which
you remove, or to add new services which your handler requires.

## Inject

I used [stephanos/inject](https://github.com/stephanos/inject) over the original
codegangsta/inject because it provides injection with factory functions as well.
This means we can transparently emulate lazy loading, just by calling `inject.Apply`.
The final object is however the same as if you'd go with the Structs example.

~~~
injector := inject.New()
injector.Map(&service.Redis{Name: "default"})
injector.Map(func() *service.Database {
	return &service.Database{Name: "default"}
})
~~~

Declaring singleton objects is just as simple as declaring lazy-loaded ones (Database).

~~~
1 import per handler, 2 in main (4 total)
0 arguments to handler (0 total)
0 lines to consume service (0 SLOC total)
1 line per service in handler declaration (1+2=3)
5 SLOC for service declaration (2.5 per service)
1 SLOC to populate services in handler (1 per handler)
1 external package
------
14 lines to create/pass 3 objects, 1 external package
2.5 lines gain per handler, 1 line in main to populate all services
~~~

The good:

1. Low increase of verbosity with increase of handlers,
2. Can use "lazy" load to populate services for handler,
3. Testable - can assign services based on handler struct,
4. Great for refactoring (adding/removing services from handlers)

The bad:

1. Handlers don't have good documentation visible from main