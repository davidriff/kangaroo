package main

import (
    "fmt"
    "os"
    "os/exec"
    "math"
    "encoding/binary"
    "io/ioutil"
    "flag"
)

func get_parameters()(string, string, int, int, int, int, int){
    encoded_file_path_ptr := flag.String("i", "", "Path to encoded file");
    output_path_ptr := flag.String("o", "decoded.file", "Path for output file");
    width_ptr := flag.Int("w", 0, "Video width");
    high_ptr := flag.Int("h", 0, "Video high");
    yuv_option_ptr := flag.Int("yuv", 3, "Use Luma Y (0), blue chroma Cb (1), red chroma Cr (2), both chromas (3) or all YCbCr (4)")
    frame_percentage_ptr := flag.Int("frame", 100, "Percentage of frames to use:10, 25, 50 or 100")
    bits_to_use_ptr := flag.Int("bits", 4, "How many bits to use in each byte: 1-8")
    flag.Parse();

    if *encoded_file_path_ptr==""{
        fmt.Println("Please, enter a valid path for encoded file\n")
        flag.PrintDefaults();
        os.Exit(1);
    }

    if *width_ptr==0{
        fmt.Println("Please, enter a valid width value\n")
        flag.PrintDefaults();
        os.Exit(1);
    }

    if *high_ptr==0{
        fmt.Println("Please, enter a valid high value\n")
        flag.PrintDefaults();
        os.Exit(1);
    }

    if *yuv_option_ptr<0 || *yuv_option_ptr>4 {
        fmt.Println("Please, enter a valid value for yuv parameter\n")
        flag.PrintDefaults();
        os.Exit(1);
    }

    if *bits_to_use_ptr<1 || *bits_to_use_ptr>8{
        fmt.Println("Please, enter a valid value for bits parameter\n")
        flag.PrintDefaults();
        os.Exit(1);
    }


    //constants of this video
    var encoded_path string = *encoded_file_path_ptr;
    var decoded_path string = *output_path_ptr;
    var width int = *width_ptr;
    var high int = *high_ptr;
    var yuv_option int = *yuv_option_ptr;
    var frame_percentage int = *frame_percentage_ptr;
    var bits_to_use int = *bits_to_use_ptr;
    var frame_increase int;

    if frame_percentage==10{
        frame_increase=10;
    }else if frame_percentage==25{
        frame_increase=4;
    }else if frame_percentage==50{
        frame_increase=2;
    }else if frame_percentage==100{
        frame_increase=1;
    }else{
        fmt.Println("Please, enter a valid value for frame percentage parameter\n");
        flag.PrintDefaults();
        os.Exit(1);
    }

    return encoded_path, decoded_path, width, high, yuv_option, frame_increase, bits_to_use
}

func check(err error) {
    if err != nil {
        fmt.Printf("ERROR: %s", err);
        os.Exit(-1);
    }
}

func read_frame(file_path string, offset int64, frame_size int) ([]byte, bool) {

    var new_position int64;
    var last_position int64;
    var end_of_file bool = false;

    read_bytes := make([]byte, frame_size)

    binary_file, err := os.Open(file_path)
    check(err)
    defer binary_file.Close()

    last_position, err = binary_file.Seek(0,2)
    check(err)

    if offset>last_position{
        end_of_file=true
        return read_bytes, end_of_file
    }

    new_position, err = binary_file.Seek(offset, 0)
    check(err)

    if new_position==last_position{//check if we reached end of video
        end_of_file=true
        return read_bytes, end_of_file
    }

    //fmt.Println(end_of_file)

    _, err = binary_file.Read(read_bytes)
    check(err)
    return read_bytes, end_of_file
}

func clear_bit(byte_in *byte, position int) { //sets to 0 the specified bit, call it by clear_bit(&variable, bit_position)
    dummy_values := [8]byte{0xfe, 0xfd, 0xfb, 0xf7, 0xef, 0xdf, 0xbf, 0x7f}
    out := *byte_in & dummy_values[position-1]
    *byte_in = out
}

func bits_to_byte(bit_array [8]byte, order int) byte{// order specifies if the bits are LittleEndian or BigEndian
    var byte_out byte = 0;
    var bit_values []byte;

    if order==0{
        bit_values=[]byte{128,64,32,16,8,4,2,1}
    }else{
        bit_values=[]byte{1,2,4,8,16,32,64,128}
    }

    for b:=0; b<8; b++{
        if bit_array[b]==1{
            byte_out=byte_out+bit_values[b];//b or 8-b ??
        }
    }
    return byte_out
}

func runcmd(cmd string) []byte {

    out, err := exec.Command("bash", "-c", cmd).Output()
    if err != nil {
        fmt.Println("\nERROR executing command: "+cmd)
    }
    return out
}

func decode_ldpc(encoded_bits []byte) []byte{

    var encoded_bits_in_ascii []byte;
    var decoded_file_ascii []byte;
    var decoded_file_bits []byte;

    for i:=0; i<len(encoded_bits); i++{

        if encoded_bits[i] == 0{
            encoded_bits_in_ascii=append(encoded_bits_in_ascii, 48);

        }else if encoded_bits[i]==1{
            encoded_bits_in_ascii=append(encoded_bits_in_ascii, 49);
        }
    }
    err := ioutil.WriteFile("encoded_bits_in_ascii", encoded_bits_in_ascii, 0644);
    check(err);

    runcmd("decode ldpc.pchk encoded_bits_in_ascii decoded-file.out bsc 0.45 enum-block ldpc.gen")
    runcmd("extract ldpc.gen decoded-file.out extracted-file")

    decoded_file_ascii, err = ioutil.ReadFile("extracted-file");

    for i:=0; i<len(decoded_file_ascii); i++{

        if decoded_file_ascii[i]==48{
            decoded_file_bits=append(decoded_file_bits,0)

        }else if decoded_file_ascii[i]==49{
            decoded_file_bits=append(decoded_file_bits,1)
        }
    }

    return decoded_file_bits
}

func extract_bits(number_of_bits_to_read uint64, encoded_path string, frame_size int, start_position int, secret_bits_per_frame int, width int, frame_increase int, bits_to_use int, zero_value byte, one_value byte) []byte{//reads frames and extracts secret bits until the number is reached

    var frame_data []byte;
    var output_data []byte;
    var sum int = 0;
    var mean float64;
    var actual_byte byte;

    var end_of_frame bool = false;

    var frame_count int = 0;
    var frame_position int;//start after Y
    var in_line_position int = 0;
    var extracted_bits uint64 = 0;
    var extracted_bits_in_frame int = 0;

    var try_1 float64;
    var try_0 float64;

    for extracted_bits!=number_of_bits_to_read{

        end_of_frame=false;
        frame_position= start_position;
        extracted_bits_in_frame = 0;

        if frame_count%frame_increase==0{//if this frame contains secret (see frame_percentage parameter), extract it

            frame_data, _=read_frame(encoded_path, int64(frame_count)*int64(frame_size), frame_size);//load frame

            for end_of_frame!=true{

                for line_position:=0; line_position<width; line_position+=width/4{
                    for line_block:=0; line_block<4; line_block++{

                        actual_byte=frame_data[frame_position+line_block+line_position+in_line_position]
                        
                        for a:=0; a<(8-bits_to_use) ;a++{
                            clear_bit(&actual_byte, 8-a);
                        }

                        sum=sum+int(actual_byte);//calculate the mean value of the 4x4 block
                    }
                }
                in_line_position=in_line_position+4;
                

                if in_line_position%(width/4)==0{//we reached the end of block-line, jump to next block-line (one block-line is made of 4 rows (320*4))
                    in_line_position=0;
                    frame_position=frame_position+width;
                }

                mean=float64(sum)/16;
                sum=0;

                try_1 = math.Abs(float64(mean)-float64(one_value));//calculate if encoded bit is 1 or 0
                try_0 = math.Abs(float64(mean)-float64(zero_value));

                if try_1 < try_0 {
                    output_data=append(output_data, 1);
                    
                }else{
                    output_data=append(output_data, 0);
                }

                extracted_bits=extracted_bits+1;
                extracted_bits_in_frame=extracted_bits_in_frame+1

                if extracted_bits_in_frame==secret_bits_per_frame{
                    end_of_frame=true;
                }

                if frame_position==frame_size{//end of frame, go to next frame
                    end_of_frame=true;
                }

                if extracted_bits==number_of_bits_to_read{//we read all we needed
                    break;
                }
            }
        }
        frame_count=frame_count+1;
    }

    //fmt.Println(output_data)
    return decode_ldpc(output_data)
}

func get_secret_size(encoded_path string, frame_size int, start_position int, secret_bits_per_frame int, width int, frame_increase int, bits_to_use int, zero_value byte, one_value byte) uint64{

    var secret_size uint64;
    var secret_size_in_bits []byte;
    var dummy_slice [] byte;
    var dummy_array [8]byte;

    var secret_size_in_bytes  []byte;

    secret_size_in_bits = extract_bits(64*10, encoded_path, frame_size, start_position, secret_bits_per_frame, width, frame_increase, bits_to_use, zero_value, one_value);//64+16*3 for the hamming bits of size header

    //reverse each bit inside each byte
    for b:=0; b<8; b++{
        dummy_slice=secret_size_in_bits[b*8:b*8+8];

        for a:=0; a<8; a++{//we need to use an array, or the original slice will be modified with each iteration
            dummy_array[a]=dummy_slice[a];
        }

        for i:=0; i<8; i++{
            secret_size_in_bits[b*8+i]=dummy_array[7-i];
        }
    }

    // turn to bytes
    for c:=0; c<8; c++{
        dummy_slice=secret_size_in_bits[c*8:c*8+8];

        for a:=0; a<8; a++{//we need to use an array, or the original slice will be modified with each iteration
            dummy_array[a]=dummy_slice[a];
        }

        secret_size_in_bytes=append(secret_size_in_bytes, bits_to_byte(dummy_array,0));
    }

    //turn to uint64
    secret_size=binary.BigEndian.Uint64(secret_size_in_bytes);

    fmt.Printf("\n%d bits to read", secret_size)
    return secret_size
}

func main() {

    var(
        encoded_path, decoded_path, width, high, yuv_option, frame_increase, bits_to_use = get_parameters()
        )

    runcmd("make-ldpc ldpc.pchk 9 10 1 evenboth 3");//build parity check matrix
    runcmd("make-gen ldpc.pchk ldpc.gen dense");//build generator matrix

    var y_size int = width*high; 
    var u_size int = width*high*2/8; //in yuv420 u_size = y_size*2/8 bytes
    var v_size int = u_size;
    var frame_size int = y_size + u_size + v_size;

    //get one_value and zero_value
    zero_values := []byte{0x00, 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40}//0, 1, 2, 4, 8, 16 , 32, 64
    one_values := []byte{0x01, 0x02, 0x05, 0x0b, 0x17, 0x2f, 0x5f, 0xbf}//1, 2, 5, 11, 23, 47, 95, 191
    var zero_value byte = zero_values[bits_to_use-1];
    var one_value byte = one_values[bits_to_use-1];

    yuv_options_size_array := []int{y_size/16, u_size/16, v_size/16, 2*u_size/16, frame_size/16}//indicates how many bits we are embeded in each frame depending on yuv_option ex-> for yuv_option 3: u_size*2/16=28800
    yuv_options_start_position_array := []int{0, y_size, y_size+u_size, y_size, 0}//indicates start position for reading (recover secret) in frame, dependending on yuv options
    var secret_bits_per_frame int = yuv_options_size_array[yuv_option];
    var start_position int = yuv_options_start_position_array[yuv_option];
    //

    var secret_in_bits []byte;
    var secret_bit_array [8]byte;
    var secret_in_bytes []byte;

    var secret_size uint64=get_secret_size(encoded_path, frame_size, start_position, secret_bits_per_frame, width, frame_increase, bits_to_use, zero_value, one_value);

    //read the secret
    fmt.Println("\n\nReading...");
    secret_in_bits=extract_bits(secret_size, encoded_path, frame_size, start_position, secret_bits_per_frame, width, frame_increase, bits_to_use, zero_value, one_value);
    fmt.Printf("\n%d bits recovered", len(secret_in_bits));

    for i:=64; i<len(secret_in_bits); i+=8{//turn bits to bytes(skip the first 64 bits which are used for secret size)

        for a:=0; a<8; a++{
            secret_bit_array[a]=secret_in_bits[i+a];
        }
        secret_in_bytes=append(secret_in_bytes, bits_to_byte(secret_bit_array,1));

    }
    err := ioutil.WriteFile(decoded_path, secret_in_bytes, 0644);
    check(err);

    //remove LPCD files
    err = os.Remove("ldpc.pchk");
    check(err);

    err = os.Remove("ldpc.gen");
    check(err);

    err = os.Remove("encoded_bits_in_ascii");
    check(err);

    err = os.Remove("decoded-file.out");
    check(err);

    err = os.Remove("extracted-file");
    check(err);
 }