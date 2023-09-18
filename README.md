# Filter proxy

This is a HTTP proxy for JSON responses with filtering capacilities.

## Make a new release

To make a new release, create a new tag and push it to the repository:

```bash
git checkout main
git tag -a 0.3.2 -m "0.3.2"
git push origin 0.3.2
```

The GitHub action will automatically create a release and attach the binaries to it.
