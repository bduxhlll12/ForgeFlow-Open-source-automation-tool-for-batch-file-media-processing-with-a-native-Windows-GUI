# Writing a plugin

1. Subclass `core.plugin_api.FlowForgePlugin`.
2. Implement `run(input_path, output_path, **options)`.
3. Call `core.plugin_api.register(YourPlugin())` on import.
