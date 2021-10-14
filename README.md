# SLSA Provenance Generator Buildkite Plugin

A proof-of-concept SLSA provenance generator for Buildkite.

It is based on [SLSA GitHub Actions Demo](https://github.com/slsa-framework/github-actions-demo),
and the following is the SLSA description from this repository:

## Background

[SLSA](https://github.com/slsa-framework/slsa) is a framework intended to codify
and promote secure software supply-chain practices. SLSA helps trace software
artifacts (e.g. binaries) back to the build and source control systems that
produced them using in-toto's
[Attestation](https://github.com/in-toto/attestation/blob/main/spec/README.md)
metadata format.

## Description

This proof-of-concept GitHub Action demonstrates an initial SLSA integration
conformant with SLSA Level 1. This provenance can be uploaded to the native
artifact store or to any other artifact repository.

While there are no integrity guarantees on the produced provenance at L1,
publishing artifact provenance in a common format opens up opportunities for
automated analysis and auditing. Additionally, moving build definitions into
source control and onto well-supported, secure build systems represents a marked
improvement from the ecosystem's current state.

## Example Usage

Generate provenance for single build artifact:

```yml
steps:
  - label: "🔨 Create artifact and generate provenance"
    command:
      - "mkdir build"
      - "echo 'build artifact' > build/artifact.txt"
    artifact_paths:
      - "build/*"
    plugins:
      - hi-artem/provenance-generator#v1.1.3:
          artifact-path: "build/artifact.txt"
          output-path: "provenance.json"
```

Generate provenance for multiple build artifacts:

```yml
steps:
  - label: "🔨 Create artifacts and generate provenance"
    command:
      - "mkdir build"
      - "echo 'build artifact 1' > build/artifact1.txt"
      - "echo 'build artifact 2' > build/artifact2.txt"
      - "echo 'build artifact 2' > build/artifact3.txt"
    artifact_paths:
      - "build/*"
    plugins:
      - hi-artem/provenance-generator#v1.1.3:
          artifact-path: "build/*"
          output-path: "provenance.json"
```

## Security and Support

This is demo repo and is not intended to be used in production contexts. 
As such, we cannot make any commitments of future support.

## Contributing

1. Fork the repo
2. Make the changes
3. Run the tests
4. Commit and push your changes
5. Send a pull request
