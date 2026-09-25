package cli

import (
	"fmt"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

func newPDFCommand() *cobra.Command {
	var filePath string

	preview := &cobra.Command{
		Use:   "preview",
		Short: "preview extracted text from a PDF",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			text, err := induction.ExtractPDFText(filePath, induction.DefaultAttachmentMaxBytes)
			if err != nil {
				return fmt.Errorf("preview PDF: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), text)
			return err
		},
	}
	preview.Flags().StringVar(&filePath, "file", "", "path to the PDF file")
	if err := preview.MarkFlagRequired("file"); err != nil {
		panic(fmt.Sprintf("mark pdf preview file as required: %v", err))
	}

	pdf := &cobra.Command{Use: "pdf", Short: "work with PDF files"}
	pdf.AddCommand(preview)
	return pdf
}
