
# About

perfma-replay is copycat of [goreplay](https://github.com/buger/goreplay), works on tcp tracffic.

It's currently a toy project and not tested as well .

All credit goes to Leonid Bugaev, [@buger](https://twitter.com/buger), https://leonsbox.com


# Author
```
    liusu

    qiyong.xie@perfma.com
   ```

# Usage

```
# Test
./perfma-replay --input-dubbo :20880 --biz-protocol "dubbo" --output-file "/Users/liusu/Downloads/request.json"
./perfma-replay --input-http :8080 --biz-protocol "http" --output-file "/Users/liusu/Downloads/request.json"
./perfma-replay --input-dubbo :20880  --biz-protocol "dubbo" --output-file "/Users/liusu/Downloads/request.json" --output-file-flush-interval "10s" --output-file-queue-limit "60000" --output-file-size-limit "32mb" --output-file-append "false"
```



