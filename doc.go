// Command print is a terminal UI for printing documents on Linux through CUPS.
//
// It lists the documents of the current directory, converts Office and plain
// text files to PDF, lets the user choose copies, page ranges, paper, ink and
// scaling, and sends the job with lp. It also exposes the queue state as JSON
// so a bar widget can show pending jobs and cancel or resume them.
//
// The command needs CUPS (lpstat, lp), LibreOffice (soffice), Poppler
// (pdfinfo), qpdf and Ghostscript (gs). See README.md for installation.
package main
