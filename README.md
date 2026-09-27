# WaveLab

WaveLab is a local HTTP service for tow-tank experiments. The original equipment and interlock pages expose a fixed commissioning diagnostic scenario. Operator measurements are a separate live data path.

Set `WAVELAB_DATA_DIR` to choose the archive directory (default `data/wavelab`). `POST /api/measured-runs` accepts `name`, `model_name`, `section`, and `carriage`. `POST /api/measured-runs/{id}/evidence` accepts `kind` (`wave_time` or `force`), `source_id`, finite `value`, and `recorded_at` in RFC3339 form. `GET /api/measured-runs`, `GET /api/measured-runs/{id}`, and `GET /api/measured-runs/{id}/analysis` return the supplied records and wave/force summary. Measurements are committed to `operator-measurements.json` before they are published in memory, and are reloaded when the service restarts. The archive does not automatically grant a safety permit or drive hardware.

The `internal/probe`, `internal/force`, `internal/permit`, `internal/carriage`, `internal/track`, `internal/wave`, `internal/absorber`, and `internal/journal` packages provide component logic for further work. Their diagnostic use does not imply that the original HTTP pages accept real sensor streams.

## Ship model custody

Physical hull models are tracked by entity number, not just by name. `POST /api/ship-models` registers a model with `id`, `name`, and `home` (`preparation`, `test_area`, or `storage`). `POST /api/ship-models/{id}/transfers` starts a handover with `to`, `seal` (`sealed`, `opened`, or `damaged`), `handed_over_by`, and an optional `run` linking the transfer to a test; `POST /api/transfers/{id}/confirm` completes it with `received_by`. `GET /api/ship-models` lists every model with its current location and pending transfer, and `GET /api/ship-models/{id}/transfers` returns the full custody chain. A model whose latest transfer has not been confirmed is in transit: any further handover is rejected with 409, so it cannot be occupied by another test. The chain is committed to `ship-model-custody.json` before it is published in memory and is reloaded on restart.
