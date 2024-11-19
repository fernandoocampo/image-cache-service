# Image Cache Service

## Test project
```
make test
```

## Run Server locally
```
make run
```

## Run a sample request against the server

synchronous call

* raw curl command
```
curl -X POST -H "Content-Type: application/json" -d @req.json http://localhost:8080/v1/resize
```
* make command
```sh
make resize-images-sync
```

asynchronous call
* one sample request
```sh
make resize-images-sync
```
* second sample request
```sh
make resize-images-sync-2
```

Now in your browser, you can check one of the returned urls!


## Assumptions

1. Project allows to upgrade go from 1.15 to 1.23 to take advantage of new features.
2. If the client does not provide any async parameters within the resize requests or their value is not a boolean, the application will assume it is a synchronous request.
3. Project allows to move logs to new library slog.
4. Calling resize image endpoint with async approach will return the image url, result "in progress" and cached in false.
5. Resize retries will be added in future versions of this API. If an async process fails in one of the URLs the image was not going to be added to the cache and it will just print an error log.
6. Items within a URL collection in the resize async request will be processed individually.
7. In the asynchronous resizing process, if the image already exists in the cache, it will return the response indicating that it is already cached.
8. I added just a couple of unit tests to validate happy path from main functions: resize [sync|async] and getImage. I usually test only public functions, following Kent Beck practice about it.
9. This implementation is a single instance deployment. 
10. I have organized project layout in order to improve loose coupling and high cohesion. I followed a package oriented style.
11. In get image endpoint Timeout or context cancellation is propagated from client.
12. I've decided that every worker that resizes an image will also be in charge of sending that result to the cache repository, in a real scenario we could have some rate limits in case we get a lot of simultaneous requests, in that case this part of the logic could be moved to the same goroutine in charge of cleaning the worker map.