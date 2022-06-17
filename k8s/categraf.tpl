---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  annotations: {}
  labels:
    app: n9e
    component: categraf
    release: nightingale
  name: nightingale-categraf
spec:
  selector:
    matchLabels:
      app: n9e
      component: categraf
      release: nightingale
  template:
    metadata:
      labels:
        app: n9e
        component: categraf
        release: nightingale
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/os
                operator: In
                values:
                - linux
      containers:
      - env:
        - name: TZ
          value: Asia/Shanghai
        - name: HOSTNAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: HOSTIP
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: status.hostIP
        - name: HOST_PROC
          value: /hostfs/proc
        - name: HOST_SYS
          value: /hostfs/sys
        - name: HOST_MOUNT_PREFIX
          value: /hostfs
        image: flashcatcloud/categraf:v0.1.3
        imagePullPolicy: IfNotPresent
        name: categraf
        resources: {}
        volumeMounts:
        - mountPath: /etc/categraf/conf/config.toml
          name: categraf-config
          subPath: config.toml
        - mountPath: /etc/categraf/conf/logs.toml
          name: categraf-config
          subPath: logs.toml
MOUNTS
        - mountPath: /var/run/utmp
          name: hostroutmp
          readOnly: true
        - mountPath: /hostfs
          name: hostrofs
          readOnly: true
        - mountPath: /var/run/docker.sock
          name: docker-socket
      dnsPolicy: ClusterFirstWithHostNet
      serviceAccountName: n9e-categraf
      hostNetwork: true
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      tolerations:
      - effect: NoSchedule
        operator: Exists
      volumes:
      - configMap:
          defaultMode: 420
          items:
          - key: config.toml
            path: config.toml
          - key: logs.toml
            path: logs.toml
          name: categraf-config
        name: categraf-config
VOLUMES
      - hostPath:
          path: /
          type: ""
        name: hostrofs
      - hostPath:
          path: /var/run/utmp
          type: ""
        name: hostroutmp
      - hostPath:
          path: /var/run/docker.sock
          type: Socket
        name: docker-socket
