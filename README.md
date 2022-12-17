# kprof

A tool to quickly get a pprof profile from a running go application running as kubernetes pod.

## Examples

**Taking a cpu profile for the specified pod**:

```bash
    kprof cpu app-cf64bc57-87bcl
```

Note: It will try the first detected port.

**Specify the container**:

```bash
    kprof cpu app-cf64bc57-87bcl -c prometheus-extractor
```

**Specify the port** the pprof http server is listening on:

```bash
    kprof cpu app-cf64bc57-87bcl -c prometheus-extractor -p 7777
```
