# mongodb

mongodb 监控采集插件，由mongodb-exporter（https://github.com/percona/mongodb_exporter） 封装而来。v0.3.30-v0.3.42从 [telegraf/mongodb](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/mongodb) fork。

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

    
### 注意事项
  > 如果MongoDB 开启了operationProfiling仅用以上权限会出现system.profile无权限错误，默认的mongo角色中对system.profile集合只有find权限，要解决这个问题需要创建一个新的角色。
  ```
  db.createRole({
    role: "StatsReader",
    privileges: [
      {
        resource: { db: "", collection: "system.profile" },
        actions: [ "collStats", "indexStats" ]
      }
    ],
    roles: []
  })

  # 给categraf用户授权
  db.grantRolesToUser("categraf",[{ role: "StatsReader", db: "admin" }])
  ```

## 监控大盘和告警规则

同级目录下的 dashboard.json、alerts.json 是大盘和告警规则, dashboard2.json 是v0.3.30版本以后的大盘。
