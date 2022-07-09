{{ range $instance := .instances }}
[[instances]]
address = "{{ $instance.address }}"
username = "{{ $instance.username }}"
password = "{{ $instance.password }}"
pool_size = "{{ $instance.pool_size }}"
labels = { instance= "{{ $instance.name }}" }
{{ end }}

