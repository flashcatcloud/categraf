namespace=test

dry_run=true
if [[ $dry_run == true ]]; then
	dry_run_params="--dry-run=client"
else
	dry_run_params=""
fi

function install() {
  #config.toml
  kubectl create cm categraf-config -n $namespace --from-literal=config.toml=../conf/config.toml --from-literal=logs.toml=../conf/logs.toml ${dry_run_params}
  
  #input.xxx
  for dir in $(find ../conf/ -maxdepth 1 -type d ! -path "../conf/" )
  do
  	name=$(echo $dir |  sed -e 's/..\/conf\///g' -e 's/\./-/g' -e 's/_/\-/g' -e 's/ //g')
  	relative=$(echo $dir | sed -e 's/\.\.\///g' -e 's/ //g')
  	kubectl create cm $name -n $namespace --from-file=$dir ${dry_run_params}
  	if [[ "X$mount" == "X" ]] ; then
  	   mount=$(echo "        - mountPath: /etc/categraf/$relative\n          name: $name")
  	else 
  	   mount=$(echo "$mount\n        - mountPath: /etc/categraf/$relative\n          name: $name")
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
  for dir in $(find ../conf/ -maxdepth 1 -type d ! -path "../conf/" )
  do
    kubectl delete cm $name -n $namespace
  done
  # daemonset
  kubectl delete ds -n $namespace nightingale-categraf

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
    * )
        usage
        ;;
esac
