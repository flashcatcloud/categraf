package update

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"k8s.io/klog/v2"
)

func download(file string) (string, error) {
	fname := filepath.Base(file)
	klog.InfoS("downloading file", "source", file, "dest", fname)
	res, err := http.Get(file)
	if err != nil {
		return fname, fmt.Errorf("cannot download file from %s", file)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fname, fmt.Errorf("download  %s error response: %s", file, res.Status)
	}
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fname, err
	}
	defer f.Close()
	bufWriter := bufio.NewWriter(f)

	_, err = io.Copy(bufWriter, res.Body)
	if err != nil {
		return fname, err
	}
	bufWriter.Flush()
	return fname, nil
}

func Update(tar string) error {
	// download
	fname, err := download(tar)
	if err != nil {
		return err
	}
	nv, err := UnTar("./", fname)
	if err != nil {
		return err
	}

	// old version
	ov, err := os.Executable()
	if err != nil {
		return err
	}
	fm, err := os.Stat(ov)
	if err != nil {
		return fmt.Errorf("stat file %s error: %s", ov, err)
	}
	fi, err := os.Stat(nv)
	if err != nil {
		return fmt.Errorf("stat file %s error: %s", nv, err)
	}
	if fi.Mode().IsDir() {
		return fmt.Errorf("%s is directory", nv)
	}
	klog.InfoS("replace old version with new version", "old_version", ov, "new_version", "./"+nv)

	// rename current -> current.old
	oldBackup := ov + ".old"
	err = os.Rename(ov, oldBackup)
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(windows.StringToUTF16Ptr(oldBackup), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT) // optional: delay delete old file
	if err != nil {
		klog.Warningf("cannot auto remove old file for current user, please manually remove %s: %v", oldBackup, err)
	}
	// replace
	err = os.Rename(nv, ov)
	if err != nil {
		return err
	}
	err = os.RemoveAll("./" + filepath.Dir(nv))
	if err != nil {
		klog.ErrorS(err, "clean dir failed", "path", "./"+filepath.Dir(nv))
	} else {
		klog.InfoS("clean dir success", "path", "./"+filepath.Dir(nv))
	}
	err = os.Remove("./" + fname)
	if err != nil {
		klog.ErrorS(err, "clean file failed", "path", "./"+fname)
	} else {
		klog.InfoS("clean file success", "path", "./"+fname)
	}
	return os.Chmod(ov, fm.Mode().Perm())
}

func UnTar(dst, src string) (target string, err error) {
	fr, err := zip.OpenReader(src)
	if err != nil {
		return
	}
	defer fr.Close()

	err = os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return "", err
	}

	// 遍历 ZIP 文件中的每个文件
	for _, file := range fr.File {
		// 构建文件解压后的路径
		destPath := filepath.Join(dst, file.Name)

		// skip directory
		if file.FileInfo().IsDir() {
			continue
		}

		// 打开 ZIP 文件中的每个文件
		srcFile, err := file.Open()
		if err != nil {
			return "", err
		}
		defer srcFile.Close()

		// now create directory for files
		err = os.MkdirAll(filepath.Dir(destPath), 0755)
		if err != nil {
			klog.ErrorS(err, "mkdir failed", "path", filepath.Base(destPath))
			return "", err
		}

		// 创建目标文件
		dest, err := os.Create(destPath)
		if err != nil {
			return "", err
		}
		defer dest.Close()

		// 将 ZIP 文件中的内容复制到目标文件
		_, err = io.Copy(dest, srcFile)
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(destPath, "categraf.exe") {
			target = destPath
		}
	}

	return target, nil
}
