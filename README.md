# minikeyvalue

[![Build Status](https://travis-ci.org/geohot/minikeyvalue.svg?branch=master)](https://travis-ci.org/geohot/minikeyvalue)

Fed up with the complexity of distributed filesystems?

minikeyvalue is a ~200 line (not including tests!) non-proxying distributed key value store. Optimized for reading files between 1MB and 1GB. Inspired by SeaweedFS, but simple. Should scale to billions of files and petabytes of data.

Even if this code is crap, the on disk format is super simple! We rely on a filesystem for blob storage. It's like the nginx cache with MD5 hashes for filenames and the real name in an xattr.

### API

- GET /key
  - Supports range requests.
  - 302 redirect to volume server.
- {PUT, DELETE} /key
  - Blocks. 200 = written, anything else = nothing happened.

### Start Master Server (default port 3000)

```
./master localhost:3001,localhost:3002 /tmp/cachedb/
```

### Start Volume Server (default port 3001)

```
./volume /tmp/volume1/
PORT=3002 ./volume /tmp/volume2/
```

### Usage

```
# put "bigswag" in key "wehave"
curl -L -X PUT -d bigswag localhost:3000/wehave

# get key "wehave" (should be "bigswag")
curl -L localhost:3000/wehave

# delete key "wehave"
curl -L -X DELETE localhost:3000/wehave
```

### Performance

```
# Fetching non-existent key: 510 req/sec
wrk -t2 -c100 -d10s http://localhost:3000/key

# Fetching existent key: 440 req/sec (idk if redirect is being followed)
wrk -t2 -c100 -d10s http://localhost:3000/wehave
```

