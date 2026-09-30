# Model manager and inspection

The model manager supports repository search, filtered file listings, GGUF
downloads, sharded files, verification, updates, removal, runtime-loaded model
listing, and missing vision-projector checks. Install the modern Hugging Face
CLI first:

```bash
pip install -U huggingface_hub
```

Model-manager commands are beta features and require `--beta`. State-changing
commands require `--yes` where indicated:

```bash
induction models search "reasoning 7B" --beta
induction models list --installed --beta
induction models inspect "author/model-GGUF" --json --beta
induction models download "author/model-GGUF" "model-Q4_K_M.gguf" --yes --beta
induction models mmproj --beta
```

The model manager requires `ModelManager.ModelsPath` in the configuration.
`induction pdf preview --file PATH` extracts local PDF text without contacting
the server.
