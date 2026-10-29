package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// Command returns the `seed` subcommand.
//
// It is safe to re-run: every fixture converges rather than failing on data
// that is already there.
func Command(app core.App) *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Create the development fixtures and print their credentials",
		Long: "Creates (or repairs) an already-verified test account so a fresh database can be\n" +
			"logged into without configured mail delivery. Safe to re-run.\n\n" +
			"The credentials are printed one KEY=value per line, so a script can read them\n" +
			"without parsing prose.",
		// The error is printed by the caller; a schema failure is not a usage
		// mistake and the usage text buries it.
		SilenceUsage: true,
		RunE: func(c *cobra.Command, _ []string) error {
			// Only `serve` applies migrations on boot, and seeding a database
			// that has never been served is the common case — the fixtures
			// need the collections to exist.
			if err := app.RunAllMigrations(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}

			result, err := Apply(app)
			if err != nil {
				return fmt.Errorf("seed: %w", err)
			}

			out := c.OutOrStdout()
			fmt.Fprintf(out, "SEED_EMAIL=%s\n", result.Email)
			fmt.Fprintf(out, "SEED_PASSWORD=%s\n", result.Password)
			fmt.Fprintf(out, "SEED_USER_ID=%s\n", result.UserID)
			fmt.Fprintf(out, "SEED_WHATSAPP_ACCOUNT_ID=%s\n", result.WhatsAppAccountID)
			fmt.Fprintf(out, "SEED_WHATSAPP_PHONE_NUMBER_ID=%s\n", result.WhatsAppPhoneNumberID)
			fmt.Fprintf(out, "SEED_WHATSAPP_WABA_ID=%s\n", result.WhatsAppWABAID)

			// Prose goes to stderr so stdout stays a clean key/value stream.
			fmt.Fprintln(c.ErrOrStderr(), "seeded a verified test account; sign in with the values above")

			return nil
		},
	}
}
