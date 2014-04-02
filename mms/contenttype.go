/*
 * Copyright 2014 Canonical Ltd.
 *
 * Authors:
 * Sergio Schvezov: sergio.schvezov@cannical.com
 *
 * This file is part of mms.
 *
 * mms is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; version 3.
 *
 * mms is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package mms

import (
	"errors"
	"fmt"
	"reflect"
)

type ContentType struct {
	Name, Type, FileName, Charset, Start, StartInfo, Domain, Path, Comment, MediaType string
	Level																			  byte
	Length, Size, CreationDate, ModificationDate, ReadDate                            uint64
	Secure                                                                            bool
	Q                                                                                 float64
	Data	[]byte
}

type DataPart struct {
	ContentType		ContentType
	Data			[]byte
}

func (dec *MMSDecoder) readQ(reflectedPdu *reflect.Value) error {
	v, err := dec.readUintVar(nil, "") 
	if err != nil {
		return err
	}
	q := float64(v)
	if q > 100 {
		q = (q - 100) / 1000
	} else {
		q = (q - 1) / 100
	}
	reflectedPdu.FieldByName("Q").SetFloat(q)
	return nil
}

func (dec *MMSDecoder) readLength(reflectedPdu *reflect.Value) (length uint64, err error) {
	switch {
	case dec.data[dec.offset+1] < 30:
		l, err := dec.readShortInteger(nil, "")
		v := uint64(l)
		reflectedPdu.FieldByName("Length").SetUint(v)
		return v, err
	case dec.data[dec.offset+1] == 31:
		dec.offset++
		l, err := dec.readUintVar(reflectedPdu, "Length")
		return l, err
	}
	return 0, fmt.Errorf("Unhandled length %#x", dec.data[dec.offset+1])
}

func (dec *MMSDecoder) readCharset(reflectedPdu *reflect.Value) error {
	var charset string

	if dec.data[dec.offset] == 127 {
		dec.offset++
		charset = "*"
	} else {
		charCode, err := dec.readInteger(nil, "")
		if err != nil {
			return err
		}
		var ok bool
		if charset, ok = CHARSETS[charCode]; !ok {
			return fmt.Errorf("Cannot find matching charset for %#x == %d", charCode, charCode)
		}
	}
	reflectedPdu.FieldByName("Charset").SetString(charset)
	return nil


}

func (dec *MMSDecoder) readMediaType(reflectedPdu *reflect.Value) (err error) {
	var mediaType string
	origOffset := dec.offset

	if mt, err := dec.readInteger(nil, ""); err == nil && len(CONTENT_TYPES) > int(mt) {
		mediaType = CONTENT_TYPES[mt]
	} else {
		err = nil
		dec.offset = origOffset
		mediaType, err = dec.readString(nil, "")
		if err != nil {
			return err
		}
	}

	reflectedPdu.FieldByName("MediaType").SetString(mediaType)
	fmt.Println("Media Type:", mediaType)
	return nil
}

func (dec *MMSDecoder) readContentTypeParts(reflectedPdu *reflect.Value) error {
	var err error
	var parts uint64
	if parts, err = dec.readUintVar(nil, ""); err != nil {
		return err
	}
	fmt.Println("Number of parts", parts)
	for i := uint64(0); i < parts; i++ {
		fmt.Println("\nPart", i, "\n")
		fmt.Printf("offset %d, value: %#x\n", dec.offset, dec.data[dec.offset])
		headerLen, err := dec.readUintVar(nil, "")
		if err != nil {
			return err
		}
		headerEnd := dec.offset + int(headerLen)
		fmt.Printf("offset %d, value: %#x\n", dec.offset, dec.data[dec.offset])
		dataLen, err := dec.readUintVar(nil, "")
		if err != nil {
			return err
		}
		fmt.Println("header len:", headerLen, "dataLen:", dataLen)
		var ct ContentType
		ctReflected := reflect.ValueOf(&ct).Elem()
		if err = dec.readContentType(&ctReflected); err != nil {
			return err
		}
		//TODO skipping non ContentType headers for now
		dec.offset = headerEnd + 3
		if _, err := dec.readBoundedBytes(&ctReflected, "Data", dec.offset + int(dataLen)); err != nil {
			return err
		}
		if ct.MediaType == "application/smil" || ct.MediaType == "text/plain" {
			fmt.Printf("%s\n", ct.Data)
		}
		fmt.Println(ct)
	}

	return nil
}

func (dec *MMSDecoder) readContentType(ctMember *reflect.Value) error {
	// Only implementing general form decoding from 8.4.2.24
	var err error
	var length uint64
	if length, err = dec.readLength(ctMember); err != nil {
		return err
	}
	fmt.Println("Content Type Length:", length)
	endOffset := int(length) + dec.offset

	if err := dec.readMediaType(ctMember); err != nil {
		return err
	}

	for dec.offset < len(dec.data) && dec.offset < endOffset {
		param, _ := dec.readInteger(nil, "")
		fmt.Printf("offset %d, value: %#x, param %#x\n", dec.offset, dec.data[dec.offset], param)
		switch param {
		case WSP_PARAMETER_TYPE_Q:
			err = dec.readQ(ctMember)
		case WSP_PARAMETER_TYPE_CHARSET:
			err = dec.readCharset(ctMember)
		case WSP_PARAMETER_TYPE_LEVEL:
			_, err = dec.readShortInteger(ctMember, "Level")
		case WSP_PARAMETER_TYPE_TYPE:
			_, err = dec.readInteger(ctMember, "Type")
		case WSP_PARAMETER_TYPE_NAME_DEFUNCT:
			fmt.Println("Name(deprecated)")
			_, err = dec.readString(ctMember, "Name")
		case WSP_PARAMETER_TYPE_FILENAME_DEFUNCT:
			fmt.Println("FileName(deprecated)")
			_, err = dec.readString(ctMember, "FileName")
		case WSP_PARAMETER_TYPE_DIFFERENCES:
			err = errors.New("Unhandled Differences")
		case WSP_PARAMETER_TYPE_PADDING:
			dec.readShortInteger(nil, "")
		case WSP_PARAMETER_TYPE_CONTENT_TYPE:
			_, err = dec.readString(ctMember, "Type")
		case WSP_PARAMETER_TYPE_START_DEFUNCT:
			fmt.Println("Start(deprecated)")
			_, err = dec.readString(ctMember, "Start")
		case WSP_PARAMETER_TYPE_START_INFO_DEFUNCT:
			fmt.Println("Start Info(deprecated")
			_, err = dec.readString(ctMember, "StartInfo")
		case WSP_PARAMETER_TYPE_COMMENT_DEFUNCT:
			fmt.Println("Comment(deprecated")
			_, err = dec.readString(ctMember, "Comment")
		case WSP_PARAMETER_TYPE_DOMAIN_DEFUNCT:
			fmt.Println("Domain(deprecated)")
			_, err = dec.readString(ctMember, "Domain")
		case WSP_PARAMETER_TYPE_MAX_AGE:
			err = errors.New("Unhandled Max Age")
		case WSP_PARAMETER_TYPE_PATH_DEFUNCT:
			fmt.Println("Path(deprecated)")
			_, err = dec.readString(ctMember, "Path")
		case WSP_PARAMETER_TYPE_SECURE:
			fmt.Println("Secure")
		case WSP_PARAMETER_TYPE_SEC:
			v, _ := dec.readShortInteger(nil, "")
			fmt.Println("SEC(deprecated)", v)
		case WSP_PARAMETER_TYPE_MAC:
			err = errors.New("Unhandled MAC")
		case WSP_PARAMETER_TYPE_CREATION_DATE:
		case WSP_PARAMETER_TYPE_MODIFICATION_DATE:
		case WSP_PARAMETER_TYPE_READ_DATE:
			err = errors.New("Unhandled Date parameters")
		case WSP_PARAMETER_TYPE_SIZE:
			_, err = dec.readInteger(ctMember, "Size")
		case WSP_PARAMETER_TYPE_NAME:
			_, err = dec.readString(ctMember, "Name")
		case WSP_PARAMETER_TYPE_FILENAME:
			_, err = dec.readString(ctMember, "FileName")
		case WSP_PARAMETER_TYPE_START:
			_, err = dec.readString(ctMember, "Start")
		case WSP_PARAMETER_TYPE_START_INFO:
			_, err = dec.readString(ctMember, "StartInfo")
		case WSP_PARAMETER_TYPE_COMMENT:
			_, err = dec.readString(ctMember, "Comment")
		case WSP_PARAMETER_TYPE_DOMAIN:
			_, err = dec.readString(ctMember, "Domain")
		case WSP_PARAMETER_TYPE_PATH:
			_, err = dec.readString(ctMember, "Path")
		case WSP_PARAMETER_TYPE_UNTYPED:
			v, _ := dec.readString(nil, "")
			fmt.Println("Untyped", v)
		default:
			err = fmt.Errorf("Unhandled parameter %#x == %d at offset %d", param, param, dec.offset)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
