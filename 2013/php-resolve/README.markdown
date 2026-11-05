Batch resolving independent data trees
--------------------------------------

Traditional development of PHP applications is not unlike a tree. Describing the
execution of a program is called an execution tree for a reason.

You load your resources, classes, create objects like database connection,
xml parser, whatever you need to satisfy the business logic of your application.
If you're among the lucky few, you have some measure of code re-use, which
allows you to change your application logic or data source in a common location
as opposed to fixing copies of the same query across your whole codebase.

Chances are you didn't hit and kind of performance issue except a slowly
performing SQL query on which you added an index to solve all your problems.

Or you might work at Facebook, Twitter, Foursquare, Tumblr, Youtube,...

One of the milestones at Facebook, now years ago, was implementing multi-get
logic into their web stack. This reduced the time taken on requests, since
a multi-get operation would complete much sooner than several sequential get
requests. The same logic can also be applied to an SQL database.

The problem with execution trees is that branches rarely know of eachother. If
they do, it's only because somebody took some logical steps in grouping the
data together, keeping track of the data being used as the program continues
down it's planned execution path, eventually resolving the data points with a
batched request.

Chances are you are not doing this.

Things get fairly complex if you're manually doing tracking of your data objects
to have them resolved in a batched way. If you're doing this I pretty much
guarantee you have an ugly codebase, or very little relationships between your
data; it is possible to get away with that.

Approaches like MVC (Model-View-Controller) are widely used today, but they
don't deal with data batching or handle re-use extensively. The principle is
sound, the only problem with it is that it's best suited for user interaction,
where the View part uses a simple Model counterpart to perform any kind of
action like reading an article or submitting a comment. When building a simple
API, the MVC works like a charm. When building a web page, you often see
several MVC components being used together.

As an example, open up www.slashdot.org and break down the page into components:

- Navigation (menu data)
- Left navigation (menu data)
- News items (main content data)
- Science side box (main content data)
- Most discussed side box (main content data)
- ...

A typical way of implementing a web page like slashdot would be to use a main
MVC model, which would give you a view of the main content on the page.
Additional MVC components would be loaded to give you the navigation, the side
navigation, the list of additional news items by section, the most discussed
contents and other site components and features.

The best I could hope for is that there is a DAO/DAL interface to provide some
common methods for fetching data like menu items and news items. I'm confident
that MVC components don't group the requests of their data together to perform
a batch request to the cache or database servers. Atleast not for the 99% of the
web pages out there. Any people who do group requests together, are left to
implement a solution themselves.

So, let's imagine a simple case. Slashdot has a navigation menu in the header
and the left column. Imagine these menus are resolved by calling a DAO method
named `GetMenuItems(section)`. How would you approach batching that call?

Possible solution: Promises

A Promise is defined as a representation of an eventual value returned from the
single completion of an operation. A promise may be one of three states -
unfulfilled, fulfilled and failed.

[Promises/A](http://wiki.commonjs.org/wiki/Promises/A) introduces a proposal
how to implement promises, which I tweaked to suit my needs. For our purposes,
I'm using "resolved" as the definitive state between a Promise which is
unfulfilled or fulfilled. When it comes to failed Promises I prefer using
Exceptions, as there is really no gain of using traditional callbacks which
dealing with nested promises.

By this, as a Promise I defined an object, with several methods:

- `getId` - Each Promise needs to have an ID which is used to resolve it's value
- `get` - Request the value of the promise - override for non batched values
- `resolve(value)` - Set the value of the promise
- `resolveAll(promises)` - The batching function to resolve a list of promises of the same type
- `getPromises()` - Return an array() of nested promisess (ie, children).

Implementing the Promises as described leaves me to pretty much keep the same
program flow, only encapsulating it in getPromises() calls, returning Promise
objects. This defines a data tree, which still needs to be resolved.

To simplify batching by specific object type, I flatten this tree using
the class name as a grouping key. I'm left with a list of object references,
which I can resolve with the grouping function resolveAll(). All the changes
to the resolved promises are reflected in this data-tree, without having to do
additional traversal.

What I'm doing after that is just transforming the Promise objects to actual
array values, which can be then passed off to your View model. If you're using
PHP to do your API responses, this data can be just passed off to your
serialization methods like json_encode() or similar.

Promises are a good way to parallelize data, but no common method exists to
batch several promises into a single call to an external data source like a
NoSQL server like Memcache or Redis, or an SQL database. By extending the model
to be data driven and batchable, I can define a data tree which is later
resolved with a batch request. If PHP supported threads, each of these batched
requests could be performed asynchronously. If you choose another language
to work with, you can still parallelize this operation.

The provided example constructs this data tree based on 4 sections. Each section
provides 4 news items without duplicates. For each news item, a comment count is
provided. Part of the data is being resolved while creating promises (eliminating
duplicate news items), and the rest is being resolved via batching requests using
`resolveAll`, and final serialization to array using `get`.

~~~
{
   "sections":{
      "1":{
         "id":1,
         "title":"Section #1",
         "items":{
            "1":{
               "id":1,
               "title":"Test title (#1)",
               "comment_count":149
            },
            "2":{
               "id":2,
               "title":"Test title (#2)",
               "comment_count":178
            },
[...snip...]
~~~

If there's a cleaner way to resolve data requirements, I haven't found it yet.
Too bad the changes aren't that straightforward for an existing piece of
software. A lot of exising programming flow would need to be reimplemented.

Given the size of the internal code base, I would be looking at refactoring
155KB just for the news module, and 1.55MB overall. Strange coincidence with
the numbers, but given the amount of code re-use, that's pretty good for
a top-tier web site. Mind you that the templates don't count into this number :)

- Tit Petric