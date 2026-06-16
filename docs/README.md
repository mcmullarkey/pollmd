# pollmd docs site

Hugo + [Hextra](https://github.com/imfing/hextra) site deployed to
`pollmd.ssp.sh` from `main` via `.github/workflows/deploy-docs.yaml`.

Most page content is pulled from the project root `README.md` through the
`readme-section` shortcode — edit there, not here, so the README stays the
source of truth.

```sh
make serve     # http://localhost:1310
make build     # static build into public/
make clean
```

Requires Hugo extended ≥ 0.156 (matches the workflow). The `go.mod` here
exists so `hugo mod` can pull the Hextra theme; it's not a Go binary.

## `README-link.md`

The `README-link.md` symlink points at `../README.md`. Hugo's `readFile`
template function is sandboxed to the project working directory, so a
symlink is the cleanest way to give the `readme-section` shortcode access
to the canonical README without duplicating content. Don't delete it.
