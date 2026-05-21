package cmd

import "fmt"

func resolveMarkdownFileFlag(contentFile, fileAlias string) (string, error) {
	if contentFile != "" && fileAlias != "" && contentFile != fileAlias {
		return "", fmt.Errorf("--content-file 与 --file 不能同时指定不同值")
	}
	if contentFile != "" {
		return contentFile, nil
	}
	return fileAlias, nil
}
