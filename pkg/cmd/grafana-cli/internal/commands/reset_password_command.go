package commands

import (
	"bufio"
	"fmt"
	"os"

	"github.com/fatih/color"
	logger "github.com/grafana/grafana/pkg/internal/infra/clilog"

	"github.com/grafana/grafana/pkg/cmd/grafana-cli/internal/utils"
	"github.com/grafana/grafana/pkg/internal/bus"
	"github.com/grafana/grafana/pkg/internal/models"
	"github.com/grafana/grafana/pkg/internal/services/sqlstore"
	"github.com/grafana/grafana/pkg/internal/util"
	"github.com/grafana/grafana/pkg/internal/util/errutil"
)

const AdminUserId = 1

func resetPasswordCommand(c utils.CommandLine, sqlStore *sqlstore.SQLStore) error {
	newPassword := ""

	if c.Bool("password-from-stdin") {
		logger.Infof("New Password: ")

		scanner := bufio.NewScanner(os.Stdin)
		if ok := scanner.Scan(); !ok {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("can't read password from stdin: %w", err)
			}
			return fmt.Errorf("can't read password from stdin")
		}
		newPassword = scanner.Text()
	} else {
		newPassword = c.Args().First()
	}

	password := models.Password(newPassword)
	if password.IsWeak() {
		return fmt.Errorf("new password is too short")
	}

	userQuery := models.GetUserByIdQuery{Id: AdminUserId}

	if err := bus.Dispatch(&userQuery); err != nil {
		return fmt.Errorf("could not read user from database. Error: %v", err)
	}

	passwordHashed, err := util.EncodePassword(newPassword, userQuery.Result.Salt)
	if err != nil {
		return err
	}

	cmd := models.ChangeUserPasswordCommand{
		UserId:      AdminUserId,
		NewPassword: passwordHashed,
	}

	if err := bus.Dispatch(&cmd); err != nil {
		return errutil.Wrapf(err, "failed to update user password")
	}

	logger.Infof("\n")
	logger.Infof("Admin password changed successfully %s", color.GreenString("✔"))

	return nil
}
