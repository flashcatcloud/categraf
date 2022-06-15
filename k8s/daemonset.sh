namespace=test

dry_run=true
if [[ $dry_run == true ]]; then
	dry_run_params="--dry-run=client"
else
	dry_run_params=""
fi

target="input.cpu input.disk input.diskio input.docker input.kernel input.kernel_vmstat input.linux_sysctl_fs input.mem input.net input.netstat input.processes input.system input.kubernetes"

function install() {
  #config.toml
  kubectl create cm categraf-config -n $namespace --from-file=../conf/config.toml --from-file=../conf/logs.toml ${dry_run_params}
  
  #input.xxx
  for dir in $(echo $target | sed 's/ /\n/g')
  do
  	name=$(echo $dir |  sed -e 's/\./-/g' -e 's/_/\-/g' -e 's/ //g')
  	relative=$(echo $dir | sed -e 's/ //g')
  	kubectl create cm $name -n $namespace --from-file=../conf/$dir ${dry_run_params}
  	if [[ "X$mount" == "X" ]] ; then
  	   mount=$(echo "        - mountPath: /etc/categraf/conf/$relative\n          name: $name")
  	else 
  	   mount=$(echo "$mount\n        - mountPath: /etc/categraf/conf/$relative\n          name: $name")
  	fi
  	if [[ "X$volume" == "X" ]]; then
  	   volume=$(echo "      - name: $name\n        configMap:\n          name: $name")
  	else
  	   volume=$(echo "$volume\n      - name: $name\n        configMap:\n          name: $name")
  	fi
  done
  
  #daemonset
  sed -e "s#MOUNTS#$mount#g" -e "s#VOLUMES#$volume#g" categraf.tpl | kubectl apply -n $namespace ${dry_run_params} -f - 
  
}

function uninstall() {
  # config.toml
  kubectl delete cm categraf-config -n $namespace ${dry_run_params}
  # input.xxx
  for dir in $(echo $target | sed 's/ /\n/g')
  do
    name=$(echo $dir |  sed -e 's/\./-/g' -e 's/_/\-/g' -e 's/ //g')
    kubectl delete cm -n $namespace $name ${dry_run_params}
  done
  # daemonset
  kubectl delete ds -n $namespace nightingale-categraf ${dry_run_params}

}

function exp() {
  echo "" > categraf.yaml
  # config.toml
  kubectl get cm categraf-config -n $namespace -o yaml | sed -e '/creationTimestamp:/d' -e '/namespace:/d' -e '/resourceVersion:/d' -e '/uid:/d' >> categraf.yaml
  # input.xxx
  for dir in $(echo $target | sed 's/ /\n/g')
  do
    name=$(echo $dir |  sed -e 's/\./-/g' -e 's/_/\-/g' -e 's/ //g')
    kubectl get cm -n $namespace $name -o yaml >> categraf.yaml
  done
  # daemonset
  kubectl get ds -n $namespace nightingale-categraf -o yaml >> categraf.yaml 

}

## usage
function usage() {
	echo "** install categraf daemonset, default namespace:test,  default action with --dry-run=client **"
	echo "usage: $0 install|uninstall"
}


action=$1
case $action in
    "install" )
        install
        ;;
    "uninstall" )
        uninstall
        ;;
    "export" )
        exp
        ;;
    * )
        usage
        ;;
esac

