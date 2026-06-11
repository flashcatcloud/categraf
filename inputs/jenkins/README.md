# Jenkins Input Plugin

This plugin collects node (Computer) status and job (Build) status from a Jenkins Continuous Integration server. 
It retrieves data by directly querying the Jenkins JSON API.

## Configuration

```toml
# Collection interval
# interval = 60

[[instances]]
# The root URL of your Jenkins service
jenkins_url = "http://localhost:8080"

# Authentication credentials (if Jenkins does not allow anonymous read access, you must provide a valid account/token)
jenkins_username = "admin"
jenkins_password = "password_or_token"

# Maximum idle connections in the TCP connection pool
# max_connections = 5
# HTTP Request Timeout
# response_timeout = "5s"

# ===== Job Filtering Options =====
# Maximum depth to scan folders/subjobs
# max_subjob_depth = 0
# Maximum number of subjobs to fetch per layer
# max_subjob_per_layer = 10
# Jobs that haven't been built within this age will be ignored
# max_build_age = "24h"

# Job name filtering, supports wildcards
# job_include = []
# job_exclude = []

# ===== Node Filtering Options =====
# Node name filtering, supports wildcards
# node_include = []
# node_exclude = []
```

## Metrics

**Global and Node Metrics:**
- `jenkins_up`: Whether the node is online (1: online, 0: offline)
- `jenkins_busy_executors`: Number of busy executors across the Jenkins cluster
- `jenkins_total_executors`: Total number of executors across the Jenkins cluster
- `jenkins_node_num_executors`: Number of executors on a specific node
- `jenkins_node_response_time`: Response time of the node
- `jenkins_node_disk_available`: Available disk space on the node
- `jenkins_node_temp_available`: Available temporary directory space on the node
- `jenkins_node_swap_available`: Available swap space on the node
- `jenkins_node_memory_available`: Available physical memory on the node
- `jenkins_node_swap_total`: Total swap on the node
- `jenkins_node_memory_total`: Total physical memory on the node

**Job Metrics:**
- `jenkins_job_duration`: Job build duration
- `jenkins_job_number`: Build number
- `jenkins_job_result_code`: Result status code of the build (0: Success, 1: Failure, 2: Not_built, 3: Unstable, 4: Aborted)
