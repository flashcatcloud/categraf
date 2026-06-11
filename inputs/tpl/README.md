# Tpl Input Plugin Template

This is **NOT** an actual input plugin.
It serves as a **Plugin Development Template**. If you want to develop a new, custom Categraf input plugin, you can simply copy the `tpl` directory, rename it to your desired plugin name, and use the existing code as a boilerplate.

## Development Guide

1. Copy `inputs/tpl` to `inputs/your_plugin_name`.
2. Change the package name to `package your_plugin_name`.
3. Modify the `inputName` constant to reflect your plugin's name.
4. Implement the logic to fetch metrics inside the `Gather(slist *types.SampleList)` function.
5. Create a corresponding configuration template under the `conf/` directory.
6. Modify the main entry file `metrics_agent.go` to anonymously import your new plugin (or configure build tags as needed).
