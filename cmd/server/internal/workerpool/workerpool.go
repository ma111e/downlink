package workerpool

// // Global pool instance
// var Pool pond.ResultPool

// // InitPool initializes the analysis worker pool with the specified number of workers
// func InitPool() {
// 	ctx := context.Background()
// 	maxWorkers := 3

// 	// Use configuration if available
// 	if config.Config.Analysis.WorkerPool != nil {
// 		if config.Config.Analysis.WorkerPool.MaxWorkers != nil {
// 			maxWorkers = *config.Config.Analysis.WorkerPool.MaxWorkers
// 		}
// 	}

// 	Pool = pond.NewResultPool(maxWorkers, pond.WithContext(ctx))

// 	log.WithField("max_workers", maxWorkers).Info("Analysis worker pool initialized")
// }
