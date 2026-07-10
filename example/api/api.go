package api

import "errors"

type Upload struct {
	ID   string
	Body []byte
}

type Server struct {
	jobs chan<- Upload
}

func New(jobs chan<- Upload) *Server { return &Server{jobs: jobs} }

//pantograph:upload kind=entry handoff-from=jobs note="queued"
func (s *Server) HandleUpload(u Upload) error {
	if err := validate(u); err != nil {
		return err
	}

	s.jobs <- u

	return nil
}

//pantograph:upload kind=decision
func validate(u Upload) error {
	if u.ID == "" || len(u.Body) == 0 {
		return errors.New("malformed upload")
	}

	return nil
}
