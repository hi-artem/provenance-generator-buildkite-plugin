# SLSA Provenance Generator Buildkite Plugin

Generates SLSA provenance for your builds

## Example

Add the following to your `pipeline.yml`:

```yml
steps:
  - command: echo 123 > file.txt
    plugins:
      - hi-artem/provenance-generator#v1.0.0:
          build-context: '{}'
          artifact-path: './file.txt'
          output-path: './provenance.json'
```

## Contributing

1. Fork the repo
2. Make the changes
3. Run the tests
4. Commit and push your changes
5. Send a pull request
