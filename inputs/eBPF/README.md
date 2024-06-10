# eBPF

利用ebpf和xdp技术进行网络数据采集

目前插件逻辑为在XDP挂载点采集RoCE网络数据，仅需设置网卡名称
后续可自定义更强大的功能
## 环境要求
本插件利用cilium提供的eBPF-go库，需要先配置eBPF环境，详细请参考[教程](https://ebpf-go.dev/guides/getting-started/),在Linux内核版本较低的机器上可能无法部署

在部署后可通过以下指令生成脚手架代码
```
go generate inputs/eBPF/eBPF.go

Compiled /home/xxx/categraf/inputs/eBPF/bpf_bpfel.o
Stripped /home/xxx/categraf/inputs/eBPF/bpf_bpfel.o
Wrote /home/xxx/categraf/inputs/eBPF/bpf_bpfel.go
Compiled /home/xxx/categraf/inputs/eBPF/bpf_bpfeb.o
Stripped /home/xxx/categraf/inputs/eBPF/bpf_bpfeb.o
Wrote /home/xxx/categraf/inputs/eBPF/bpf_bpfeb.go
```
利用以上脚手架代码进行eBPF相关操作