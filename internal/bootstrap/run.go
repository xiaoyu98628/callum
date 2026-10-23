package bootstrap

import "context"

type runner interface {
	Run(ctx context.Context) error
}

func Run(err error) {

}
