# mtail Input Plugin

## Introduction
Function: Extracts content from log files and converts them into monitoring metrics.

+ **Input**: Log files
+ **Output**: Metrics generated according to `mtail` syntax (only `counter`, `gauge`, and `histogram` are supported).
+ **Processing**: Essentially regex extraction and expression calculation in Golang.

## Startup
Edit the `mtail.toml` file. Generally, each instance needs to specify different `progs` (different `mtail` program files or directories) to prevent metrics from interfering with each other.

**Note**: If different instances share the same `progs`, you can differentiate them by adding `labels` to each instance:
```toml
labels = { k1="v1" }
```
Or:
```toml
[instances.labels]
k1="v1"
```

1. Specify the instance in `conf/input.mtail/mtail.toml`:
```toml
[[instances]]
## Directory containing mtail progs
progs = "/path/to/prog1"
## Log files for mtail to read
logs = ["/path/to/a.log", "path/to/b.log"] 
## Specify timezone overrides if necessary
# override_timezone = "Asia/Shanghai" 
## Whether metrics include a timestamp (note: this is a string "true")
# emit_metric_timestamp = "true" 
## Log encoding (gbk, gb18030, gb2312, big5, utf-8), default is utf-8
# encoding = "gbk" 
...
```

2. Write a rule file in the `/path/to/prog1` directory (e.g. `test.mtail`):
```text
gauge xxx_errors
/ERROR.*/ {
    xxx_errors++
}
```

3. Open a terminal tab and run `categraf --test --inputs mtail` to test.
4. In another tab, append an `ERROR` line to `/path/to/a.log` and observe Categraf's output.
5. Once testing passes, start Categraf normally.

### Input
The `logs` parameter specifies the log sources. It supports glob matching and multiple log files.

### Processing Rules
`progs` specifies the specific rule file directory (or file).

## Processing Rules and Syntax

### Processing Workflow
```python 
for line in lines:
  for regex in regexes:
    if match:
      do something
```

### Syntax Overview

```text
exported variable 

pattern { 
  action statements
} 

def decorator { 
  pattern and action statements
}
```

#### Defining Metric Names
Only `counter`, `gauge`, and `histogram` types are supported.

Example:
```text
counter lines
/INFO.*/ {
    lines++
}
```

Note: Defined names only support C-style naming conventions (letters/numbers/underscores). **If you want to use hyphens "-", use `as` to export an alias**. For example:
```text
counter lines_total as "line-count"
```
The exported metric name will be `line-count`.

#### Matching and Calculation (pattern/action)

```text
PATTERN {
  ACTION
}
```

Support for RE2 regular expressions:
```text
const PREFIX /^\w+\W+\d+ /

PREFIX {
  ACTION1
}

PREFIX + /foo/ {
  ACTION2
}
```

#### Relational Operators
- `<` less than, `<=` less than or equal to
- `>` greater than, `>=` greater than or equal to
- `==` equal to, `!=` not equal to
- `=~` match (regex), `!~` does not match (regex)
- `||` logical OR, `&&` logical AND, `!` logical NOT

#### Mathematical Operators
- `|` bitwise OR, `&` bitwise AND, `^` bitwise XOR
- `+ - * /` basic arithmetic
- `<<` bitwise left shift, `>>` bitwise right shift
- `**` exponentiation
- `=` assignment
- `++` increment, `--` decrement
- `+=` add and assign

#### Supporting `else` and `otherwise`
```text
/foo/ {
  ACTION1
} else {
  ACTION2
}
```

Nested blocks and `otherwise` are supported:
```text
/foo/ {
  /foo1/ {
     ACTION1
  }
  otherwise {
     ACTION3
  }
}
```

#### Named and Unnamed Extraction
```text
/(?P<operation>\S+) (\S+) \[\S+\] (\S+) \(\S*\) \S+ (?P<bytes>\d+)/ {
  bytes_total[$operation][$3] += $bytes
}
```

Adding constant labels:
```text
hidden text env
env="production"
counter line_total by logfile,env
/^(?P<date>\w+\s+\d+\s+\d+:\d+:\d+)/ {
    line_total[getfilename()][env]++
}
```

To add variable labels to metrics, you **must** use named extraction:
```text
# Log content
192.168.0.1 GET /foo

# test.mtail
counter my_http_requests_total by log_file, verb 
/^/ +
/(?P<host>[0-9A-Za-z\.:-]+) / +
/(?P<verb>[A-Z]+) / +
/(?P<URI>\S+).*/ +
/$/ {
    my_http_requests_total[getfilename()][$verb]++
}
```

Named extraction variables can be used in conditions:
```text
/(?P<x>\d+)/ && $x > 1 {
  nonzero_positives++
}
```

#### Time Processing
By default, the system time is used for metrics (`emit_metric_timestamp="false"`).
If you set `emit_metric_timestamp="true"`, Categraf will attach timestamps.

You can also parse timestamps from log lines:
```text
histogram http_latency buckets 1, 2, 4, 8
/^(?P<date>\w+\s+\d+\s+\d+:\d+:\d+)/ {
    strptime($date, "Jan 02 15:04:05")
	/latency=(?P<latency>\d+)/ {
		http_latency=$latency
	}
}
```

Pay attention to timezones when extracting time from logs. Use `override_timezone` to control timezone parsing. For example, setting `override_timezone="Asia/Shanghai"` ensures that the extracted time is treated as East 8 timezone and properly converted to timestamp.
