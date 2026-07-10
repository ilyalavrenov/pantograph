package worker

import "github.com/ilyalavrenov/pantograph/example/api"

type Store interface {
	Save(u api.Upload) error
}

type Worker struct {
	jobs  <-chan api.Upload
	store Store
}

func New(jobs <-chan api.Upload, store Store) *Worker {
	return &Worker{jobs: jobs, store: store}
}

//pantograph:upload kind=process handoff-to=jobs
func (w *Worker) Process() error {
	for u := range w.jobs {
		if err := w.save(u); err != nil {
			return err
		}
	}

	return nil
}

//pantograph:upload kind=store
func (w *Worker) save(u api.Upload) error {
	return w.store.Save(u)
}
