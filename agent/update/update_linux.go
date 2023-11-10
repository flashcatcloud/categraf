package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func download(file string) (string, error) {
	fname := filepath.Base(file)
	log.Println("downloading file:", file, "save to:", fname)
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
		return err
	}
	fi, err := os.Stat(nv)
	if err != nil {
		return err
	}
	if fi.Mode().IsDir() {
		return fmt.Errorf("%s is directory", nv)
	}
	log.Printf("I! replace old version:%s with new version:%s", ov, "./"+nv)

	// replace
	err = os.Rename(nv, ov)
	if err != nil {
		return err
	}
	err = os.RemoveAll("./" + filepath.Dir(nv))
	if err != nil {
		log.Println("E! clean dir:", "./"+filepath.Dir(nv), "error:", err)
	} else {
		log.Println("I! clean dir:", "./"+filepath.Dir(nv), "success")
	}
	err = os.Remove("./" + fname)
	if err != nil {
		log.Println("E! clean file:", "./"+fname, "error:", err)
	} else {
		log.Println("I! clean file:", "./"+fname, "success")
	}
	return os.Chmod(ov, fm.Mode().Perm())
}

func UnTar(dst, src string) (target string, err error) {
	fr, err := os.Open(src)
	if err != nil {
		return
	}
	defer fr.Close()

	gr, err := gzip.NewReader(fr)
	if err != nil {
		return
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()

		switch {
		case err == io.EOF:
			return target, nil
		case err != nil:
			return target, err
		case hdr == nil:
			continue
		}

		dstFileDir := filepath.Join(dst, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if b := ExistDir(dstFileDir); !b {
				if err := os.MkdirAll(dstFileDir, 0775); err != nil {
					return target, err
				}
			}
		case tar.TypeReg:
			err := os.MkdirAll(filepath.Dir(dstFileDir), 0755)
			if err != nil {
				log.Printf("mdkir:%s, error:%s", filepath.Base(dstFileDir), err)
				return target, err
			}
			file, err := os.OpenFile(dstFileDir, os.O_CREATE|os.O_RDWR, os.FileMode(hdr.Mode))
			if err != nil {
				return target, err
			}
			if strings.HasSuffix(dstFileDir, "categraf") {
				target = dstFileDir
			}
			_, err = io.Copy(file, tr)
			if err != nil {
				return target, err
			}
			file.Close()
		}
	}

	return target, nil
}

func ExistDir(dirname string) bool {
	fi, err := os.Stat(dirname)
	return (err == nil || os.IsExist(err)) && fi.IsDir()
}
