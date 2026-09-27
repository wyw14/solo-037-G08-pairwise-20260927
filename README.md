# WaveLab

WaveLab is a local HTTP service for tow-tank experiments. The original equipment and interlock pages expose a fixed commissioning diagnostic scenario. Operator measurements are a separate live data path.

Set `WAVELAB_DATA_DIR` to choose the archive directory (default `data/wavelab`). `POST /api/measured-runs` accepts `name`, `model_name`, `section`, and `carriage`. `POST /api/measured-runs/{id}/evidence` accepts `kind` (`wave_time` or `force`), `source_id`, finite `value`, and `recorded_at` in RFC3339 form. `GET /api/measured-runs`, `GET /api/measured-runs/{id}`, and `GET /api/measured-runs/{id}/analysis` return the supplied records and wave/force summary. Measurements are committed to `operator-measurements.json` before they are published in memory, and are reloaded when the service restarts. The archive does not automatically grant a safety permit or drive hardware.

The `internal/probe`, `internal/force`, `internal/permit`, `internal/carriage`, `internal/track`, `internal/wave`, `internal/absorber`, and `internal/journal` packages provide component logic for further work. Their diagnostic use does not imply that the original HTTP pages accept real sensor streams.

## Ship model custody

Ship models are registered by a unique entity number via `POST /api/ship-models` (`entity_no`, `name`, optional `note`). `GET /api/ship-models` lists every model with its current location (`preparation`, `testing`, or `storage`), any unconfirmed handover, and the active run occupying it.

Each physical handover is recorded in the model's custody chain: `POST /api/ship-models/{entity}/handovers` with `to`, `packaging` (`sealed`, `opened`, `repacked`), `handed_over_by`, and — required when `to` is `testing` — the associated `run`. The receiver confirms with `POST /api/handovers/{id}/confirm` (`received_by`). `GET /api/ship-models/{entity}/handovers` returns the full chain. The ledger is committed to `ship-model-custody.json` before it is published in memory and is reloaded on restart.

`POST /api/measured-runs` accepts an optional `model_entity` binding the run to a registered model. A model cannot be bound to a new run while a handover awaits confirmation or while another active run occupies it (HTTP 409). `POST /api/measured-runs/{id}/close` (`outcome`: `complete` or `abort`) ends a run and releases its model.
