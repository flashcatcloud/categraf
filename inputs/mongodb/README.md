# mongodb

mongodb 监控采集插件，v0.3.30开始从telegraf/mongodb fork而来，支持mongodb 3.6+版本。

## Configuration


    
- 配置文件，[参考示例](../../conf/input.mongodb/mongodb.toml)
- 配置权限，至少授予以下权限给配置文件中用于连接 MongoDB 的 user 才能收集指标：
    ```
    {
         "role":"clusterMonitor",
         "db":"admin"
      },
      {
         "role":"read",
         "db":"local"
      }

    ```
  一个简单配置
    ```
    mongo -h xxx -u xxx -p xxx --authenticationDatabase admin
    > use admin
    > db.createUser({user:"categraf",pwd:"categraf",roles: [{role:"read",db:"local"},{"role":"clusterMonitor","db":"admin"}]})
    ```
    更详细的权限配置请参考[官方文档](https://www.mongodb.com/docs/manual/reference/built-in-roles/#mongodb-authrole-clusterMonitor)

## 监控大盘和告警规则

同级目录下的 dashboard.json、alerts.json 是大盘和告警规则, dashboard2.json 是v0.3.30版本以后的大盘。
