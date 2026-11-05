# Nginx proxy_cache invalidation

The code here is presented as a two-article series on the scene-si.org blog:

1. [Purging cached items from NGINX with LUA](https://scene-si.org/2016/11/02/purging-cached-items-from-nginx-with-lua/)
2. Wildcard purge of cached items with NGINX and LUA (TODO)

The development of the wildcard purge was sponsored by Carlos Mirando Molina, based on the
discussion in the [reddit thread of the original post](https://www.reddit.com/r/nginx/comments/5apd4u/purging_cached_items_from_nginx_with_lua_tit/).
A big thanks to him for supporting open source development.

# Updates since the original version

Both the purge-single.lua and purge-multi.lua now return a `X-Purged-Count` header, depending on how many files were
found and deleted. For purge-single, the value is 0 if the item was not in the cache, and 1 if it was found. For
purge-multi, the value will be the exact number of items found, which may be 0 to N (possibly several thousands).

# Caveat emptor

## Wildcard purge (purge-multi.lua)

Purging items in such a manner requires `find`, `grep` and `xargs`. Depending on the amount of cached files that you have,
and the hardware/VM resources you have available, a wildcard purge operation might be very expensive. With that in mind,
you can consider many variants to optimize cache use.

The complexity of the operation is `O(N+M)`, where N is the number of items in a cache, and M the number of items
that must be purged from the cache. O(N) is done with an exec call (syscall, process), and O(M) is done with LUA code.

With this in mind:

1. Limit cache sizes, the more files you have, the slower it will be,
2. Try to split up caches (many caches mean smaller purges),
3. Wildcard purge is a heavy disk IO operation (pays to have SSD)

## Single purge (purge-single.lua)

Purging items in such a manner doesn't issue any syscalls apart from the deletion of the file. The complexity of this
operation is `O(1)`. The request may have a minimal CPU impact, as a MD5 hash needs to be calculated.
